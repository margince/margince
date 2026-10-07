"""The three wire shapes the e2e-llm bridge speaks to a model, stdlib only.

modules/ai in the Go tree already speaks the OpenAI-compatible wire for the
product; this is a Python copy because the lane runs on a fresh checkout with
neither the Go toolchain nor an SDK, and it only ever drives a test.

Each adapter keeps its own conversation history, in its vendor's own shape, so
what goes back to the model between tool turns (reasoning items included) is
exactly what that vendor sent.
"""

import collections
import http.client
import json
import os
import ssl
import time
import urllib.error
import urllib.request

# text: what the model said this turn; calls: [(id, tool, arguments as a JSON
# string)]; usage: Transcript.add_usage's dict, or None when the reply had none.
Turn = collections.namedtuple("Turn", "text calls usage")

ATTEMPTS = 3
TIMEOUT = 300


# Where an operating system keeps its CA bundle: macOS, Debian/Ubuntu, Fedora.
SYSTEM_CA_BUNDLES = (
    "/etc/ssl/cert.pem",
    "/etc/ssl/certs/ca-certificates.crt",
    "/etc/pki/tls/certs/ca-bundle.crt",
)


def tls_context(default_store_empty=None):
    """A verifying TLS context that also works on python.org's macOS build.

    That build ships no CA certificates until its installer script is run, so
    every vendor call fails verification and the lane stops before a model is
    asked. Verification is never relaxed: an empty store borrows the system's.
    """
    context = ssl.create_default_context()
    context.minimum_version = ssl.TLSVersion.TLSv1_2
    if default_store_empty is None:
        default_store_empty = context.cert_store_stats()["x509_ca"] == 0
    if default_store_empty:
        for bundle in SYSTEM_CA_BUNDLES:
            if os.path.exists(bundle):
                context.load_verify_locations(cafile=bundle)
                break
    return context


_TLS = tls_context()


class ProviderFault(Exception):
    """The model could not be asked: a refused credential, a rejected request,
    or a vendor still failing after retries. A harness fault, never scored."""


def _post(vendor, url, headers, body, sleep):
    last = ""
    for attempt in range(ATTEMPTS):
        request = urllib.request.Request(
            url, json.dumps(body).encode(), dict(headers, **{"Content-Type": "application/json"}),
            method="POST",
        )
        try:
            with urllib.request.urlopen(request, timeout=TIMEOUT, context=_TLS) as response:
                raw = response.read()
            reply = _decoded(raw)
            # A broker carries an upstream failure in a 200, and a proxy answers
            # with a page: both are as transient as the 5xx they stand for.
            if isinstance(reply, dict) and "error" not in reply:
                return reply
            last = f"HTTP 200 carrying {str(reply if reply is not None else raw[:200])[:300]}"
        except urllib.error.HTTPError as err:
            detail = err.read().decode("utf-8", "replace")[:300]
            # A 4xx other than 429 will answer the same way every time.
            if err.code != 429 and err.code < 500:
                raise ProviderFault(f"{vendor} refused the request: HTTP {err.code}: {detail}") from err
            last = f"HTTP {err.code}: {detail}"
        except (urllib.error.URLError, TimeoutError, ConnectionError, http.client.HTTPException) as err:
            last = repr(err)
        if attempt < ATTEMPTS - 1:
            sleep(2 ** (attempt + 1))
    raise ProviderFault(f"{vendor} kept failing after {ATTEMPTS} attempts: {last}")


class _Wire:
    """step() turns a reply whose shape this adapter does not know into a
    ProviderFault, so a vendor changing a field stops the lane rather than
    crashing the bridge into a run the lane would score."""

    def step(self, results):
        try:
            return self._step(results)
        except (KeyError, IndexError, TypeError, AttributeError) as err:
            raise ProviderFault(f"{self._route.candidate} answered in a shape the bridge does not read: {err!r}") from err


# Anthropic caches only up to a marked block, and at most four of them. The
# system prompt is marked (the tools ahead of it are cached with it) and so is
# the newest message, which rolls forward each turn — marks are added to the
# request, never to the stored history, so they cannot accumulate.
_EPHEMERAL = {"type": "ephemeral"}


def _marked(message):
    """A copy of `message` whose last content block is a cache breakpoint."""
    content = message["content"]
    blocks = [{"type": "text", "text": content}] if isinstance(content, str) else [dict(b) for b in content]
    if not blocks:
        return message
    blocks[-1] = dict(blocks[-1], cache_control=_EPHEMERAL)
    return dict(message, content=blocks)


def _decoded(raw):
    """The reply as JSON, or None when it is not JSON at all."""
    try:
        return json.loads(raw)
    except ValueError:
        return None


def _effort(route):
    """The pinned reasoning effort, or None where the candidate pins none."""
    kind, _, level = route.effort.partition(" ")
    return level if kind == "reasoning" else None


class _Chat(_Wire):
    """OpenAI-compatible Chat Completions: Mistral directly, anything via OpenRouter."""

    def __init__(self, route, key, sleep):
        self._route, self._key, self._sleep = route, key, sleep

    def start(self, system, prompt, tools):
        self._messages = ([{"role": "system", "content": system}] if system else []) + [
            {"role": "user", "content": prompt}
        ]
        self._tools = [
            {"type": "function", "function": {
                "name": tool["name"], "description": tool.get("description", ""),
                "parameters": tool.get("inputSchema") or {"type": "object"},
            }}
            for tool in tools
        ]

    def _step(self, results):
        for call_id, text, _is_error in results or ():
            self._messages.append({"role": "tool", "tool_call_id": call_id, "content": text})
        messages = list(self._messages)
        # Through a broker only Claude needs the marks; GPT and Mistral cache
        # a repeated prefix on their own.
        if self._route.candidate == "claude":
            if messages[0]["role"] == "system":
                messages[0] = _marked(messages[0])
            messages[-1] = _marked(messages[-1])
        body = {"model": self._route.model, "messages": messages}
        # No parallel_tool_calls: the vendor's default already allows them, and
        # a broker routing on require_parameters drops any endpoint lacking it.
        if self._tools:
            body["tools"] = self._tools
        if _effort(self._route):
            body["reasoning"] = {"effort": _effort(self._route)}
        if self._route.via == "openrouter":
            body["usage"] = {"include": True}
            if self._route.routing:
                body["provider"] = self._route.routing
        reply = _post(self._route.candidate, f"{self._route.base_url}/chat/completions",
                      {"Authorization": f"Bearer {self._key}"}, body, self._sleep)
        message = reply["choices"][0]["message"]
        content = message.get("content") or ""
        # Some models return content as typed chunks rather than one string.
        if isinstance(content, list):
            content = "".join(c.get("text", "") for c in content if c.get("type") == "text")
        calls = [
            (c["id"], c["function"]["name"], c["function"].get("arguments") or "{}")
            for c in message.get("tool_calls") or ()
        ]
        echoed = {"role": "assistant", "content": content}
        if message.get("tool_calls"):
            echoed["tool_calls"] = message["tool_calls"]
        if message.get("reasoning_details"):
            echoed["reasoning_details"] = message["reasoning_details"]
        self._messages.append(echoed)
        return Turn(content, calls, self._usage(reply.get("usage")))

    @staticmethod
    def _usage(usage):
        if not usage:
            return None
        details = usage.get("prompt_tokens_details") or {}
        cached = details.get("cached_tokens") or 0
        written = details.get("cache_write_tokens") or 0
        return {
            "input": usage["prompt_tokens"] - cached - written, "output": usage["completion_tokens"],
            "cache_read": cached, "cache_write": written,
            "cost": usage.get("cost"),
        }


class _Responses(_Wire):
    """OpenAI's Responses API, which carries GPT's reasoning between tool turns."""

    def __init__(self, route, key, sleep):
        self._route, self._key, self._sleep = route, key, sleep

    def start(self, system, prompt, tools):
        self._system = system
        self._input = [{"role": "user", "content": prompt}]
        self._tools = [
            {"type": "function", "name": tool["name"], "description": tool.get("description", ""),
             "parameters": tool.get("inputSchema") or {"type": "object"}, "strict": False}
            for tool in tools
        ]

    def _step(self, results):
        for call_id, text, _is_error in results or ():
            self._input.append({"type": "function_call_output", "call_id": call_id, "output": text})
        # store=false keeps nothing at OpenAI, so reasoning travels back to the
        # model encrypted inside the input instead.
        body = {"model": self._route.model, "input": self._input, "store": False,
                "include": ["reasoning.encrypted_content"]}
        if self._system:
            body["instructions"] = self._system
        if self._tools:
            body.update(tools=self._tools, parallel_tool_calls=True)
        if _effort(self._route):
            body["reasoning"] = {"effort": _effort(self._route)}
        reply = _post(self._route.candidate, f"{self._route.base_url}/responses",
                      {"Authorization": f"Bearer {self._key}"}, body, self._sleep)
        output = reply.get("output") or []
        self._input.extend(output)
        text = "".join(
            part.get("text", "")
            for item in output if item.get("type") == "message"
            for part in item.get("content") or () if part.get("type") == "output_text"
        )
        calls = [(i["call_id"], i["name"], i.get("arguments") or "{}")
                 for i in output if i.get("type") == "function_call"]
        return Turn(text, calls, self._usage(reply.get("usage")))

    @staticmethod
    def _usage(usage):
        if not usage:
            return None
        cached = (usage.get("input_tokens_details") or {}).get("cached_tokens") or 0
        return {"input": usage["input_tokens"] - cached, "output": usage["output_tokens"],
                "cache_read": cached, "cache_write": 0, "cost": None}


class _Messages(_Wire):
    """Anthropic's Messages API."""

    def __init__(self, route, key, sleep):
        self._route, self._key, self._sleep = route, key, sleep

    def start(self, system, prompt, tools):
        self._system = system
        self._messages = [{"role": "user", "content": prompt}]
        self._tools = [
            {"name": tool["name"], "description": tool.get("description", ""),
             "input_schema": tool.get("inputSchema") or {"type": "object"}}
            for tool in tools
        ]

    def _step(self, results):
        if results:
            self._messages.append({"role": "user", "content": [
                {"type": "tool_result", "tool_use_id": call_id, "content": text, "is_error": is_error}
                for call_id, text, is_error in results
            ]})
        messages = self._messages[:-1] + [_marked(self._messages[-1])]
        body = {"model": self._route.model, "max_tokens": 8192, "messages": messages}
        if self._system:
            body["system"] = [{"type": "text", "text": self._system, "cache_control": _EPHEMERAL}]
        if self._tools:
            body["tools"] = self._tools
        reply = _post(self._route.candidate, f"{self._route.base_url}/v1/messages",
                      {"x-api-key": self._key, "anthropic-version": "2023-06-01"}, body, self._sleep)
        blocks = reply.get("content") or []
        self._messages.append({"role": "assistant", "content": blocks})
        text = "".join(b.get("text", "") for b in blocks if b.get("type") == "text")
        calls = [(b["id"], b["name"], json.dumps(b.get("input") or {}))
                 for b in blocks if b.get("type") == "tool_use"]
        return Turn(text, calls, self._usage(reply.get("usage")))

    @staticmethod
    def _usage(usage):
        if not usage:
            return None
        return {"input": usage["input_tokens"], "output": usage["output_tokens"],
                "cache_read": usage.get("cache_read_input_tokens") or 0,
                "cache_write": usage.get("cache_creation_input_tokens") or 0, "cost": None}


_WIRES = {"chat": _Chat, "responses": _Responses, "messages": _Messages}


def adapter(route, key, sleep=time.sleep):
    if route.wire not in _WIRES:
        raise ProviderFault(f"the bridge does not drive the {route.wire} wire")
    return _WIRES[route.wire](route, key, sleep)
