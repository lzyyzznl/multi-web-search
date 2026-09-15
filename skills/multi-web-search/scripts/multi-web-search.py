#!/usr/bin/env python3
"""multi-web-search 的 Python 回落实现 —— 与 Go 二进制（src/）行为对齐。

用途：技能目录里没有就地二进制时（拷贝丢文件、未支持的平台、只想跑个脚本），
启动器 `scripts/multi-web-search` 会自动回落到本文件。因此本文件与 Go 实现
**共享同一套磁盘状态**，语义必须逐字对齐：

  * 缓存      ~/.cache/multi-web-search/<sha256(query)>.json，TTL 15 分钟
  * 熔断      ~/.config/multi-web-search/circuit.json，429/403 立即熔断 24h，
              瞬时错误连续 3 次熔断 24h
  * 引擎名    serper / baidu / brave / tavily / aliyun-iqs / exa
  * 权重      serper 1.0 / brave 0.9 / exa 0.9 / tavily 0.85 / baidu 0.8 / aliyun-iqs 0.75
  * 去重      URL 归一化后取最高分，engine_scores 记录各引擎得分
  * 输出结构  {query, meta{total_raw,total_unique,duration_ms}, engine_status, results}

与二进制不一致的地方只有一处：`key add` 需要平台级环境变量持久化，回落实现
不做，直接给出提示（见 cmd_key）。

环境变量：MULTI_WEB_SEARCH_NO_PROXY=1 强制直连（与二进制一致）。
"""

from __future__ import annotations

import argparse
import hashlib
import json
import os
import re
import socket
import sys
import time
import urllib.error
import urllib.parse
import urllib.request
from concurrent.futures import ThreadPoolExecutor, wait
from datetime import datetime, timedelta, timezone
from pathlib import Path

VERSION = "dev-python-fallback"

# 引擎名 → API Key 环境变量（与 src/cmd/env.go 一致）
ENV_VAR_MAP = {
    "serper": "SERPER_API_KEY",
    "baidu": "BAIDU_API_KEY",
    "brave": "BRAVE_API_KEY",
    "tavily": "TAVILY_API_KEY",
    "aliyun-iqs": "ALIYUN_IQS_API_KEY",
    "exa": "EXA_API_KEY",
}

# 引擎名 → 排序权重（与 src/cmd/merge.go 一致）
ENGINE_WEIGHT = {
    "serper": 1.0,
    "brave": 0.9,
    "exa": 0.9,
    "tavily": 0.85,
    "baidu": 0.8,
    "aliyun-iqs": 0.75,
}

# 引擎名 → 单次 HTTP 超时秒数（与各 engine 的 http.Client.Timeout 一致）
ENGINE_TIMEOUT = {"serper": 15, "brave": 15, "baidu": 20, "tavily": 30, "aliyun-iqs": 30, "exa": 20}

SEARCH_TIMEOUT = 8  # 整体超时（src/cmd/search.go searchTimeout）
DEFAULT_NUM = 10  # 每引擎默认条数
CACHE_TTL = 15 * 60  # 缓存有效期
MAX_CONSECUTIVE_FAILURES = 3  # 瞬时错误熔断阈值
CIRCUIT_OPEN_HOURS = 24  # 熔断时长
PROXY_PORTS = ("10808", "10809", "7890", "7897", "1080", "8081", "8888")  # src/cmd/proxy.go
USER_AGENT = "multi-web-search/1.0 (python-fallback)"


class EngineError(Exception):
    """引擎级错误。message 与 Go 侧 error.Error() 保持同构（前缀为引擎名）。"""


# ------------------------------------------------------------------ 磁盘路径


def home_dir() -> Path:
    return Path(os.path.expanduser("~"))


def cache_dir() -> Path:
    path = home_dir() / ".cache" / "multi-web-search"
    path.mkdir(parents=True, exist_ok=True)
    return path


def circuit_path() -> Path:
    path = home_dir() / ".config" / "multi-web-search"
    path.mkdir(parents=True, exist_ok=True)
    return path / "circuit.json"


def write_atomic(path: Path, data: str) -> None:
    tmp = path.with_name(path.name + ".tmp")
    tmp.write_text(data, encoding="utf-8")
    os.replace(tmp, path)


def _rfc3339(moment: datetime) -> str:
    return moment.isoformat(timespec="microseconds")


def _parse_rfc3339(text: str) -> datetime:
    try:
        return datetime.fromisoformat(text)
    except ValueError:
        return datetime.fromtimestamp(0, tz=timezone.utc)


# ------------------------------------------------------------------ 代理检测


def _port_alive(host: str, port: int, timeout: float = 0.15) -> bool:
    try:
        with socket.create_connection((host, port), timeout=timeout):
            return True
    except OSError:
        return False


def _registry_proxy() -> str:
    """读 Windows 注册表 ProxyEnable/ProxyServer，多协议格式优先 https 段。"""
    if os.name != "nt":
        return ""
    try:
        import winreg
    except ImportError:
        return ""
    try:
        with winreg.OpenKey(winreg.HKEY_CURRENT_USER,
                            r"Software\Microsoft\Windows\CurrentVersion\Internet Settings") as key:
            enabled, _ = winreg.QueryValueEx(key, "ProxyEnable")
            server, _ = winreg.QueryValueEx(key, "ProxyServer")
    except OSError:
        return ""
    if not enabled or not server:
        return ""
    if "=" in server:
        parts = dict(item.split("=", 1) for item in server.split(";") if "=" in item)
        server = parts.get("https") or parts.get("http") or ""
    if not server:
        return ""
    port = server.rsplit(":", 1)[-1]
    if port.isdigit() and not _port_alive("127.0.0.1", int(port)):
        return ""
    return f"http://{server}"


def detect_proxy() -> str:
    """返回可用代理地址；空串表示直连（对齐 src/cmd/proxy.go）。"""
    if os.environ.get("MULTI_WEB_SEARCH_NO_PROXY") == "1":
        return ""
    for name in ("HTTPS_PROXY", "https_proxy"):
        if os.environ.get(name):
            return os.environ[name]
    if os.name == "nt":
        found = _registry_proxy()
        if found:
            return found
        return ""
    for port in PROXY_PORTS:
        if _port_alive("127.0.0.1", int(port)):
            return f"http://127.0.0.1:{port}"
    return ""


_PROXY_OPENER: urllib.request.OpenerDirector | None = None


def opener() -> urllib.request.OpenerDirector:
    global _PROXY_OPENER
    if _PROXY_OPENER is None:
        proxy = detect_proxy()
        handler = urllib.request.ProxyHandler({"http": proxy, "https": proxy} if proxy
                                             else {"http": None, "https": None})
        _PROXY_OPENER = urllib.request.build_opener(handler)
    return _PROXY_OPENER


def http_json(url: str, *, method: str = "POST", headers: dict | None = None, body: dict | None = None,
              timeout: float = 20) -> dict:
    data = json.dumps(body).encode("utf-8") if body is not None else None
    request = urllib.request.Request(url, data=data, method=method,
                                     headers={"Content-Type": "application/json",
                                              "User-Agent": USER_AGENT, **(headers or {})})
    with opener().open(request, timeout=timeout) as response:
        return json.loads(response.read().decode("utf-8", "replace"))


def http_error_text(engine: str, exc: urllib.error.HTTPError) -> str:
    """把 HTTP 错误转成与 Go 侧同构的消息（含状态码字面量，供熔断分类匹配）。"""
    if exc.code == 429:
        return f"{engine}: 429 rate limit"
    detail = exc.read().decode("utf-8", "replace")[:300]
    return f"{engine}: HTTP {exc.code}: {detail}"


# ------------------------------------------------------------------ 六个引擎


def _rows(items, title_key, url_key, snippet_key, source, score_key=None, score_fn=None) -> list[dict]:
    results = []
    for index, item in enumerate(items or []):
        link = (item.get(url_key) or "").strip()
        if not link:
            continue
        score = 0.0
        if score_key:
            try:
                score = float(item.get(score_key) or 0)
            except (TypeError, ValueError):
                score = 0.0
        if score <= 0 and score_fn:
            score = score_fn(index)
        results.append({"title": (item.get(title_key) or "").strip(), "url": link,
                        "snippet": (item.get(snippet_key) or "").strip(),
                        "source": source, "score": score})
    return results


def search_serper(query: str, num: int) -> list[dict]:
    key = os.environ.get(ENV_VAR_MAP["serper"], "")
    if not key:
        raise EngineError("serper: API Key 未配置")
    try:
        data = http_json("https://google.serper.dev/search", headers={"X-API-KEY": key},
                         body={"q": query, "num": num}, timeout=ENGINE_TIMEOUT["serper"])
    except urllib.error.HTTPError as exc:
        raise EngineError(http_error_text("serper", exc)) from None
    except Exception as exc:
        raise EngineError(f"serper: {exc}") from None
    return _rows(data.get("organic"), "title", "link", "snippet", "serper")


def search_baidu(query: str, num: int) -> list[dict]:
    key = os.environ.get(ENV_VAR_MAP["baidu"], "")
    if not key:
        raise EngineError("baidu: API Key 未配置")
    body = {"messages": [{"content": query, "role": "user"}], "search_source": "baidu_search_v2",
            "resource_type_filter": [{"type": "web", "top_k": num}]}
    try:
        data = http_json("https://qianfan.baidubce.com/v2/ai_search/web_search",
                         headers={"Authorization": f"Bearer {key}", "X-Appbuilder-From": "openclaw"},
                         body=body, timeout=ENGINE_TIMEOUT["baidu"])
    except urllib.error.HTTPError as exc:
        raise EngineError(http_error_text("baidu", exc)) from None
    except Exception as exc:
        raise EngineError(f"baidu: {exc}") from None
    code = data.get("code") or ""
    if code and code != "200":
        raise EngineError(f"baidu: API error code={code}")
    return _rows(data.get("references"), "title", "url", "content", "baidu")


def search_brave(query: str, num: int) -> list[dict]:
    key = os.environ.get(ENV_VAR_MAP["brave"], "")
    if not key:
        raise EngineError("brave: API Key 未配置")
    url = "https://api.search.brave.com/res/v1/web/search?" + urllib.parse.urlencode(
        {"q": query, "count": num})
    try:
        data = http_json(url, method="GET", headers={"X-Subscription-Token": key,
                                                     "Accept": "application/json"},
                         timeout=ENGINE_TIMEOUT["brave"])
    except urllib.error.HTTPError as exc:
        raise EngineError(http_error_text("brave", exc)) from None
    except Exception as exc:
        raise EngineError(f"brave: {exc}") from None
    return _rows((data.get("web") or {}).get("results"), "title", "url", "description", "brave")


def search_tavily(query: str, num: int) -> list[dict]:
    key = os.environ.get(ENV_VAR_MAP["tavily"], "")
    if not key:
        raise EngineError("tavily: API Key 未配置")
    try:
        data = http_json("https://api.tavily.com/search",
                         body={"api_key": key, "query": query, "max_results": num},
                         timeout=ENGINE_TIMEOUT["tavily"])
    except urllib.error.HTTPError as exc:
        raise EngineError(http_error_text("tavily", exc)) from None
    except Exception as exc:
        raise EngineError(f"tavily: {exc}") from None
    return _rows(data.get("results"), "title", "url", "content", "tavily", score_key="score")


def search_aliyun_iqs(query: str, num: int) -> list[dict]:
    key = os.environ.get(ENV_VAR_MAP["aliyun-iqs"], "")
    if not key:
        raise EngineError("aliyun-iqs: API Key 未配置")
    body = {"query": query, "engineType": "Generic", "timeRange": "NoLimit", "pageSize": num,
            "contents": {"mainText": False, "markdownText": False, "summary": True, "rerankScore": True}}
    try:
        data = http_json("https://cloud-iqs.aliyuncs.com/search/unified",
                         headers={"Authorization": f"Bearer {key}"}, body=body,
                         timeout=ENGINE_TIMEOUT["aliyun-iqs"])
    except urllib.error.HTTPError as exc:
        raise EngineError(http_error_text("aliyun-iqs", exc)) from None
    except Exception as exc:
        raise EngineError(f"aliyun-iqs: {exc}") from None
    results = []
    for item in (data.get("pageItems") or [])[:num]:
        snippet = (item.get("summary") or "").strip() or (item.get("snippet") or "").strip()
        link = (item.get("link") or "").strip()
        if not link:
            continue
        results.append({"title": (item.get("title") or "").strip(), "url": link, "snippet": snippet,
                        "source": "aliyun-iqs",
                        "score": float(item.get("rerankScore") or 0)})
    return results


def search_exa(query: str, num: int) -> list[dict]:
    key = os.environ.get(ENV_VAR_MAP["exa"], "")
    if not key:
        raise EngineError("exa: API Key 未配置")
    body = {"query": query, "numResults": num, "type": "auto",
            "contents": {"text": {"maxCharacters": 300}}}
    try:
        data = http_json("https://api.exa.ai/search", headers={"Authorization": f"Bearer {key}"},
                         body=body, timeout=ENGINE_TIMEOUT["exa"])
    except urllib.error.HTTPError as exc:
        raise EngineError(http_error_text("exa", exc)) from None
    except Exception as exc:
        raise EngineError(f"exa: {exc}") from None
    return _rows(data.get("results"), "title", "url", "text", "exa", score_key="score",
                 score_fn=lambda i: max(0.1, 0.8 - 0.05 * i))


ENGINES = {
    "serper": search_serper,
    "baidu": search_baidu,
    "brave": search_brave,
    "tavily": search_tavily,
    "aliyun-iqs": search_aliyun_iqs,
    "exa": search_exa,
}


def enabled_engines() -> list[str]:
    return sorted(name for name, env in ENV_VAR_MAP.items() if os.environ.get(env))


# ------------------------------------------------------------------ 去重排序


def normalize_url(raw_url: str) -> str:
    """URL 归一化：scheme→https、去 www.、去末尾斜杠、去 fragment、去 utm_* 参数。"""
    try:
        parts = urllib.parse.urlsplit(raw_url)
    except ValueError:
        return raw_url
    host = parts.hostname or ""
    if parts.port:
        host = f"{host}:{parts.port}"
    if host.startswith("www."):
        host = host[4:]
    if not host:
        return raw_url
    path = parts.path[:-1] if parts.path.endswith("/") else parts.path
    pairs = [(k, v) for k, v in urllib.parse.parse_qsl(parts.query, keep_blank_values=True)
             if not k.lower().startswith("utm_")]
    query = urllib.parse.urlencode(sorted(pairs))
    return f"https://{host}{path}" + (f"?{query}" if query else "")


def merge_results(all_results: list[list[dict]]) -> list[dict]:
    """按归一化 URL 去重；同 URL 取最高分，engine_scores 记录各引擎得分（对齐 merge.go）。"""
    seen: dict[str, dict] = {}
    for results in all_results:
        for item in results:
            key = normalize_url(item["url"])
            weight = ENGINE_WEIGHT.get(item["source"], 0.0)
            base = item["score"] if item["score"] > 0 else 0.5
            score = weight * base
            existing = seen.get(key)
            if existing is None:
                merged = dict(item, score=score, engine_scores={item["source"]: score})
                seen[key] = merged
                continue
            if score > existing["score"]:
                existing["score"] = score
            existing["engine_scores"][item["source"]] = score
    for entry in seen.values():
        # Go 的 encoding/json 会按 key 排序输出 map，这里在合并阶段就定序
        entry["engine_scores"] = dict(sorted(entry["engine_scores"].items()))
    return sorted(seen.values(), key=lambda item: item["score"], reverse=True)


# ------------------------------------------------------------------ 缓存


def cache_file(query: str) -> Path:
    return cache_dir() / (hashlib.sha256(query.encode("utf-8")).hexdigest() + ".json")


def _tidy_result(item: dict) -> dict:
    """把结果的 engine_scores 定序（Go 序列化 map 时按 key 排序）。"""
    if isinstance(item.get("engine_scores"), dict):
        item["engine_scores"] = dict(sorted(item["engine_scores"].items()))
    return item


def load_cache(query: str) -> dict | None:
    path = cache_file(query)
    try:
        entry = json.loads(path.read_text(encoding="utf-8"))
    except (OSError, json.JSONDecodeError):
        return None
    cached_at = _parse_rfc3339(entry.get("cached_at", ""))
    if datetime.now(cached_at.tzinfo) - cached_at > timedelta(seconds=CACHE_TTL):
        return None
    results = [_tidy_result(dict(item)) for item in entry.get("results") or []]
    return {"query": entry.get("query", query),
            "meta": {"total_raw": entry.get("raw", 0), "total_unique": len(results),
                     "duration_ms": 0},
            "engine_status": None, "results": results}


def save_cache(query: str, response: dict) -> None:
    entry = {"query": query, "results": response["results"], "raw": response["meta"]["total_raw"],
             "cached_at": _rfc3339(datetime.now().astimezone())}
    write_atomic(cache_file(query), json.dumps(entry, ensure_ascii=False))


# ------------------------------------------------------------------ 熔断


GO_ZERO_TIME = "0001-01-01T00:00:00Z"  # Go time.Time 零值，闭合态条目里会出现


def load_circuit() -> dict:
    try:
        return json.loads(circuit_path().read_text(encoding="utf-8"))
    except (OSError, json.JSONDecodeError):
        return {}


def _go_duration(seconds: float) -> str:
    """按 Go time.Duration.String() 的形态输出（截断到秒）。"""
    total = max(0, int(seconds))
    hours, rest = divmod(total, 3600)
    minutes, secs = divmod(rest, 60)
    if hours:
        return f"{hours}h{minutes}m{secs}s"
    if minutes:
        return f"{minutes}m{secs}s"
    return f"{secs}s"


def _circuit_entry(store: dict, engine: str) -> dict:
    """取出的条目补齐 Go 侧字段（含零值），保证回写后文件形态一致。"""
    entry = dict(store.get(engine) or {})
    entry.setdefault("state", "closed")
    entry.setdefault("open_at", GO_ZERO_TIME)
    entry.setdefault("open_until", GO_ZERO_TIME)
    entry.setdefault("reason", "")
    entry.setdefault("consecutive_failures", 0)
    entry.setdefault("last_error", "")
    return entry


def _circuit_persist(entry: dict) -> dict:
    """按 Go 的 json tag 形态落盘：空 last_error / 零 consecutive_failures 不写。"""
    out = {"state": entry.get("state", "closed"),
           "open_at": entry.get("open_at", GO_ZERO_TIME),
           "open_until": entry.get("open_until", GO_ZERO_TIME),
           "reason": entry.get("reason", "")}
    if entry.get("last_error"):
        out["last_error"] = entry["last_error"]
    if entry.get("consecutive_failures"):
        out["consecutive_failures"] = entry["consecutive_failures"]
    return out


def save_circuit(store: dict) -> None:
    try:
        write_atomic(circuit_path(), json.dumps(store, ensure_ascii=False, indent=2))
    except OSError:
        pass


def circuit_is_open(store: dict, engine: str) -> bool:
    entry = _circuit_entry(store, engine)
    if entry["state"] == "open":
        if datetime.now().astimezone() > _parse_rfc3339(entry["open_until"]):
            entry["state"] = "closed"
            store[engine] = _circuit_persist(entry)
            save_circuit(store)
            return False
        return True
    return False


def circuit_status_text(store: dict, engine: str) -> str:
    entry = _circuit_entry(store, engine)
    if entry["state"] == "closed":
        return "ok"
    remaining = (_parse_rfc3339(entry["open_until"]) - datetime.now().astimezone()).total_seconds()
    return f"熔断中 (剩余 {_go_duration(remaining)}) — {entry['reason']}"


def circuit_open(store: dict, engine: str, reason: str, last_error: str) -> None:
    now = datetime.now().astimezone()
    entry = _circuit_entry(store, engine)
    entry.update({"state": "open", "open_at": _rfc3339(now),
                  "open_until": _rfc3339(now + timedelta(hours=CIRCUIT_OPEN_HOURS)),
                  "reason": reason, "last_error": last_error})
    store[engine] = _circuit_persist(entry)
    save_circuit(store)


def circuit_record_failure(store: dict, engine: str, last_error: str) -> None:
    entry = _circuit_entry(store, engine)
    entry["consecutive_failures"] = int(entry.get("consecutive_failures") or 0) + 1
    entry["last_error"] = last_error
    if entry["consecutive_failures"] >= MAX_CONSECUTIVE_FAILURES:
        now = datetime.now().astimezone()
        entry.update({"state": "open", "open_at": _rfc3339(now),
                      "open_until": _rfc3339(now + timedelta(hours=CIRCUIT_OPEN_HOURS)),
                      "reason": "consecutive_failures", "consecutive_failures": 0})
    store[engine] = _circuit_persist(entry)
    save_circuit(store)


def circuit_record_success(store: dict, engine: str) -> None:
    entry = _circuit_entry(store, engine)
    if entry.get("consecutive_failures"):
        entry["consecutive_failures"] = 0
        store[engine] = _circuit_persist(entry)
        save_circuit(store)


def is_quota_error(message: str) -> bool:
    return any(token in message for token in ("429", "403", "quota", "rate limit", "insufficient_quota"))


def is_transient_error(message: str) -> bool:
    for code in ("500", "501", "502", "503", "504", "505", "508", "521", "522", "523", "524", "529", "530"):
        if f"HTTP {code}" in message:
            return True
    lowered = message.lower()
    return any(token in lowered for token in ("timeout", "timed out", "deadline exceeded", "connection",
                                              "refused", "unreachable", "temporary", "eof"))


# ------------------------------------------------------------------ 搜索编排


def do_search(query: str, nums: int, engines: list[str], timeout: int, no_cache: bool) -> dict:
    if not no_cache:
        cached = load_cache(query)
        if cached:
            return cached

    candidates = enabled_engines()
    if engines:
        wanted = set(engines)
        candidates = [name for name in candidates if name in wanted]
    if not candidates:
        raise SystemExit("未检测到任何搜索引擎的 API Key，请至少配置一个环境变量或指定引擎")

    num = nums if nums > 0 else DEFAULT_NUM
    limit = timeout if timeout > 0 else SEARCH_TIMEOUT
    store = load_circuit()
    started = time.time()
    engine_status: dict[str, dict] = {}
    collected: list[list[dict]] = []

    def run(engine: str) -> tuple[str, int, list[dict] | str]:
        begin = time.time()
        try:
            results = ENGINES[engine](query, num)
        except EngineError as exc:
            return engine, int((time.time() - begin) * 1000), str(exc)
        except Exception as exc:  # 网络层兜底，消息形态与 Go 的 err 对齐
            return engine, int((time.time() - begin) * 1000), f"{engine}: {exc}"
        return engine, int((time.time() - begin) * 1000), results

    pending = []
    for name in candidates:
        if circuit_is_open(store, name):
            engine_status[name] = {"status": "circuit_open", "results": 0, "latency_ms": 0,
                                   "error": circuit_status_text(store, name)}
            continue
        pending.append(name)

    if pending:
        with ThreadPoolExecutor(max_workers=len(pending)) as pool:
            futures = {pool.submit(run, name): name for name in pending}
            done, not_done = wait(futures, timeout=limit)
            for future in done:
                name, latency, payload = future.result()
                if isinstance(payload, str):
                    if "API Key 未配置" not in payload:
                        if is_quota_error(payload):
                            circuit_open(store, name, "quota_exhausted", payload)
                        elif is_transient_error(payload):
                            circuit_record_failure(store, name, payload)
                    engine_status[name] = {"status": "error", "results": 0, "latency_ms": latency,
                                           "error": payload}
                    continue
                circuit_record_success(store, name)
                collected.append(payload)
                engine_status[name] = {"status": "ok", "results": len(payload), "latency_ms": latency}
            for future in not_done:
                name = futures[future]
                future.cancel()
                message = f"{name}: context deadline exceeded"
                circuit_record_failure(store, name, message)
                engine_status[name] = {"status": "error", "results": 0,
                                       "latency_ms": int(limit * 1000), "error": message}

    merged = merge_results(collected)
    response = {
        "query": query,
        "meta": {"total_raw": sum(status["results"] for status in engine_status.values()),
                 "total_unique": len(merged),
                 "duration_ms": int((time.time() - started) * 1000)},
        "engine_status": engine_status,
        "results": merged,
    }
    if not no_cache:
        save_cache(query, response)
    return response


# ------------------------------------------------------------------ 输出


def truncate(text: str, max_chars: int) -> str:
    return text if len(text) <= max_chars else text[:max_chars] + "..."


def print_pretty(response: dict) -> None:
    statuses = response.get("engine_status") or {}
    print(f'\n搜索 "{response["query"]}" — 来自 {len(statuses)} 个引擎\n')
    for name, status in statuses.items():
        if status["status"] == "ok":
            print(f"  🔵 {name:<12} ({status['results']} 条, {status['latency_ms']}ms)")
        elif status["status"] == "circuit_open":
            print(f"  ⚪ {name:<12} 跳过 ({status['error']})")
        else:
            print(f"  🔴 {name:<12} 失败 ({status['error']})")
    print()
    for index, item in enumerate(response["results"], 1):
        print(f"  {index}. {item['title']}")
        print(f"     {item['url']}")
        print(f"     {truncate(item['snippet'], 120)}  [{item['score']:.2f} | {item['source']}]")
        print()
    print(f"  📊 总计 {response['meta']['total_raw']} 条原始结果，"
          f"去重后 {response['meta']['total_unique']} 条唯一结果 "
          f"(耗时 {response['meta']['duration_ms']}ms)")


def cmd_status() -> int:
    store = load_circuit()
    print("🔍 搜索引擎状态")
    for name in sorted(ENV_VAR_MAP):
        key = os.environ.get(ENV_VAR_MAP[name], "")
        if not key:
            print(f"  ❌ {name:<12} 未配置 ({ENV_VAR_MAP[name]})")
            continue
        text = circuit_status_text(store, name)
        if text == "ok":
            print(f"  ✅ {name:<12} 已配置，状态正常")
        else:
            print(f"  ⚠️  {name:<12} {text}")
    return 0


def cmd_key(args) -> int:
    print("错误：key 子命令需要就地二进制（负责平台级环境变量持久化）", file=sys.stderr)
    print("      请使用 scripts/multi-web-search.exe key add <engine> <api_key>", file=sys.stderr)
    print("      或手动设置环境变量：" +
          "、".join(f"{name}→{env}" for name, env in sorted(ENV_VAR_MAP.items())), file=sys.stderr)
    return 1


# ------------------------------------------------------------------ CLI


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(prog="multi-web-search", add_help=True)
    parser.add_argument("-v", "--version", action="version",
                        version=f"multi-web-search version {VERSION}")
    sub = parser.add_subparsers(dest="command")

    search = sub.add_parser("search")
    search.add_argument("query")
    search.add_argument("--json", action="store_true")
    search.add_argument("--raw", action="store_true")
    search.add_argument("--num", type=int, default=0)
    search.add_argument("--engines", action="append", default=[])
    search.add_argument("--timeout", type=int, default=0)
    search.add_argument("--no-cache", action="store_true")

    sub.add_parser("status")

    key = sub.add_parser("key")
    key_sub = key.add_subparsers(dest="key_command")
    key_add = key_sub.add_parser("add")
    key_add.add_argument("engine")
    key_add.add_argument("api_key")
    return parser


def dump_json(response: dict) -> None:
    """输出 JSON：2 空格缩进 + Go 默认的 HTML 转义（< > &）+ LF，与二进制逐字节一致。"""
    text = json.dumps(response, ensure_ascii=False, indent=2)
    text = text.replace("&", "\\u0026").replace("<", "\\u003c").replace(">", "\\u003e")
    sys.stdout.write(text + "\n")


def main(argv: list[str] | None = None) -> int:
    parser = build_parser()
    args = parser.parse_args(argv)

    if args.command == "search":
        engines: list[str] = []
        for chunk in args.engines:
            engines.extend(part.strip().lower() for part in chunk.split(",") if part.strip())
        unknown = [name for name in engines if name not in ENV_VAR_MAP]
        if unknown:
            print(f"Error: 未知引擎 {', '.join(unknown)}，可用: {', '.join(sorted(ENV_VAR_MAP))}",
                  file=sys.stderr)
            return 1
        response = do_search(args.query, args.num, engines, args.timeout, args.no_cache)
        if args.json or args.raw:
            dump_json(response)
        else:
            print_pretty(response)
        return 0
    if args.command == "status":
        return cmd_status()
    if args.command == "key":
        return cmd_key(args)
    parser.print_help()
    return 1


if __name__ == "__main__":
    for stream in (sys.stdout, sys.stderr):
        if hasattr(stream, "reconfigure"):
            # newline="\n"：Windows 上也不要把 \n 翻成 \r\n，与 Go 输出保持逐字节一致
            stream.reconfigure(encoding="utf-8", newline="\n")
    sys.exit(main())
