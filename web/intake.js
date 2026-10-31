/* 요청 접수함 — 관리 수첩의 한 기능판.
 * 하는 일은 하나입니다: 받은 요청을 7칸으로 정리하고, 빈칸만 되묻고, 근무일 2일 안에 담당 리드에게 넘깁니다.
 * 7칸은 요청자가 요청 창구(ask.html, 따로 연 포트)에 직접 적습니다. 여기서 옮겨 적는 것은 말로 받은 요청뿐입니다.
 * 계산(근무일 시계·빈칸·문장)은 web/core.js(NBCore)를 그대로 쓰고, 저장은 전체 수첩과 같은 REST API를 씁니다.
 * CSP: 인라인 스크립트·style 속성을 쓰지 않습니다.
 */
(function () {
  "use strict";
  var K = window.NBCore;
  var STATUSES = ["접수", "정리 중", "리드 전달", "진행", "완료", "보류", "종료"];
  var S = { raw: {}, settings: {}, requests: [], programs: [], filter: "open", open: null, ready: false, loaded: false, demo: false, only: "", version: "", auth: false, leaving: false, ask: { on: false, urls: [] }, seen: null };

  /* ---------- 작은 도우미 ---------- */
  function $(id) { return document.getElementById(id); }
  function h(tag, attrs) {
    var n = document.createElement(tag);
    if (attrs) for (var k in attrs) {
      if (!Object.prototype.hasOwnProperty.call(attrs, k)) continue;
      var v = attrs[k];
      if (k === "text") n.textContent = v == null ? "" : String(v);
      else if (k === "cls") n.className = v;
      else if (k.slice(0, 2) === "on" && typeof v === "function") n.addEventListener(k.slice(2), v);
      else if (k === "style") continue;
      else if (v !== undefined && v !== null && v !== false) n.setAttribute(k, v === true ? "" : v);
    }
    var kids = Array.prototype.slice.call(arguments, 2);
    (function put(list) { list.forEach(function (c) { if (c == null || c === false) return; if (Array.isArray(c)) put(c); else n.append(c.nodeType ? c : String(c)); }); })(kids);
    return n;
  }
  function arr(x) { return Array.isArray(x) ? x.filter(function (d) { return d && typeof d === "object" && d.id != null; }) : []; }
  function pad2(n) { return String(n).padStart(2, "0"); }
  function toDate(x) { var d = x ? new Date(x) : null; return d && !isNaN(d) ? d : null; }
  function fmtDT(x) { var d = toDate(x); return d ? (d.getMonth() + 1) + "/" + d.getDate() + " (" + K.WD[d.getDay()] + ") " + pad2(d.getHours()) + ":" + pad2(d.getMinutes()) : "—"; }
  function fmtDay(x) { var d = toDate(x); return d ? (d.getMonth() + 1) + "/" + d.getDate() : ""; }
  function localInput(x) { var d = toDate(x) || new Date(); return K.ymd(d) + "T" + pad2(d.getHours()) + ":" + pad2(d.getMinutes()); }
  function fromLocal(v) { var d = v ? new Date(v) : null; return d && !isNaN(d) ? d.toISOString() : ""; }
  function pill(cls, text) { return h("span", { cls: "pill " + cls, text: text }); }
  function lines(s) { return String(s || "").split(/\r?\n/).map(function (x) { return x.trim(); }).filter(Boolean); }
  function calNow() { return K.cal(S.settings.holidays); }
  function progName(id) { return id ? K.programLabel(S.programs, id) : ""; }
  function roles() { var r = Array.isArray(S.settings.requesterRoles) ? S.settings.requesterRoles.filter(Boolean) : []; return r.length ? r : K.REQUESTER_ROLES; }

  var toastTimer = null;
  function toast(msg) { var el = $("toast"); el.textContent = msg; el.hidden = false; clearTimeout(toastTimer); toastTimer = setTimeout(function () { el.hidden = true; }, 3600); }
  function storeNote(msg, retry) {
    var n = $("store-note");
    if (!msg) { n.hidden = true; n.replaceChildren(); return; }
    n.hidden = false; n.className = "callout store crit";
    n.replaceChildren(h("p", { text: msg }));
    if (retry) n.append(h("div", { cls: "row" }, h("button", { cls: "btn sm", type: "button", onclick: function () { storeNote(""); refresh(); } }, "다시 연결")));
  }

  /* ---------- REST(전체 수첩과 같은 API) ---------- */
  function goLogin() { if (S.leaving) return; S.leaving = true; location.href = "/login"; }
  function api(method, path, body) {
    var init = { method: method, headers: { "Accept": "application/json" }, credentials: "same-origin", cache: "no-store" };
    if (body !== undefined) { init.headers["Content-Type"] = "application/json"; init.body = typeof body === "string" ? body : JSON.stringify(body); }
    return fetch(path, init).then(function (res) {
      if (res.status === 401) { goLogin(); throw { code: "auth", message: "로그인이 필요합니다." }; }
      if (res.status === 204) return null;
      return res.text().then(function (txt) {
        var data = null;
        if (txt) { try { data = JSON.parse(txt); } catch (e) { data = null; } }
        if (!res.ok) throw { code: (data && data.error) || ("http_" + res.status), message: (data && data.message) || "" };
        return data;
      });
    }, function () { throw { code: "network", message: "" }; });
  }
  function docPath(coll, id) { return "/api/docs/" + coll + "/" + encodeURIComponent(id); }
  function failed(e) {
    if (e && e.code === "auth") return;
    toast(e && e.message ? e.message : "저장하지 못했습니다. 서버가 켜져 있는지 확인해 주세요.");
    refresh();
  }
  function write(p) { return p.then(function (r) { return refresh().then(function () { return r; }); }, failed); }
  function add(coll, doc) { return write(api("POST", "/api/docs/" + coll, doc)); }
  function upd(coll, id, patch) { return write(api("PATCH", docPath(coll, id), patch)); }
  function del(coll, id) { return write(api("DELETE", docPath(coll, id))); }
  function putSettings(patch) { return write(api("PUT", "/api/settings", Object.assign({}, S.raw, patch))); }

  var refreshing = null;
  function refresh() {
    if (refreshing) return refreshing;
    refreshing = api("GET", "/api/state").then(applyState, function (e) {
      if (e && e.code === "auth") return;
      storeNote(S.loaded ? "서버에서 새 기록을 읽지 못했습니다. 잠시 뒤 다시 연결해 주세요." : "서버에 연결하지 못했습니다. 수첩이 켜져 있는지 확인한 뒤 다시 연결해 주세요. 지금 적는 것은 저장되지 않습니다.", true);
      setReady(false);
    }).then(function () { refreshing = null; });
    return refreshing;
  }
  function applyState(st) {
    st = st || {};
    S.raw = st.settings && typeof st.settings === "object" ? st.settings : {};
    S.settings = K.withDefaults(S.raw);
    S.requests = arr(st.requests);
    S.programs = K.sortedPrograms(arr(st.programs));
    S.version = st.version ? String(st.version) : S.version;
    S.demo = st.demo === true;
    S.only = typeof st.only === "string" ? st.only : "";
    S.auth = st.auth === true;
    var ask = st.ask && typeof st.ask === "object" ? st.ask : {};
    S.ask = { on: ask.on === true, port: ask.port || 0, urls: Array.isArray(ask.urls) ? ask.urls.filter(function (u) { return typeof u === "string" && u; }) : [] };
    /* 창구로 새 요청이 들어오면 알립니다(처음 읽을 때는 빼고). */
    var ids = {}, fresh = 0;
    S.requests.forEach(function (r) { ids[r.id] = 1; if (S.seen && !S.seen[r.id] && r.via === "창구") fresh++; });
    S.seen = ids;
    if (fresh) toast("창구로 새 요청 " + fresh + "건이 들어왔습니다.");
    S.loaded = true;
    storeNote(""); setReady(true);
    render();
  }
  function setReady(on) { S.ready = !!on; $("ib-add").disabled = !on; }

  /* ---------- 복사 ---------- */
  function legacyCopy(text) {
    var ta = h("textarea", { cls: "offscreen", "aria-hidden": "true", readonly: true }); ta.value = text;
    document.body.append(ta); ta.select();
    var ok = false; try { ok = document.execCommand("copy"); } catch (e) { ok = false; }
    ta.remove(); return ok;
  }
  function copyBox(text) {
    var old = $("copy-box"); if (old) old.remove();
    var ta = h("textarea", { rows: 6, readonly: true, "aria-label": "복사할 글" }); ta.value = text;
    var box = h("div", { id: "copy-box", cls: "copy-box", role: "dialog", "aria-label": "직접 복사" },
      h("p", { cls: "ex", text: "자동 복사가 막혀 있습니다. 아래 글이 선택되어 있으니 Ctrl+C로 복사한 뒤 닫아 주세요." }), ta,
      h("div", { cls: "row end" }, h("button", { cls: "btn", type: "button", text: "닫기", onclick: function () { box.remove(); } })));
    document.body.append(box); ta.focus(); ta.select();
  }
  function copyText(text, done) {
    var ok = function () { toast(done || "복사했습니다."); };
    var fail = function () { if (legacyCopy(text)) ok(); else copyBox(text); };
    try {
      if (navigator.clipboard && navigator.clipboard.writeText) navigator.clipboard.writeText(text).then(ok, fail);
      else fail();
    } catch (e) { fail(); }
  }

  /* ---------- 머리 숫자 ---------- */
  function renderCounts(now) {
    var k = K.intakeCounts(S.requests, now, calNow());
    function box(n, label, cls) { return h("div", { cls: "ib-count" + (cls ? " " + cls : "") }, h("b", { text: String(n) }), h("span", { text: label })); }
    $("ib-counts").replaceChildren(
      box(k.open, "열린 요청"),
      box(k.over, "기한 넘김", k.over ? "crit" : ""),
      box(k.blanks, "빈칸 있음", k.blanks ? "warn" : ""));
    document.title = (k.over ? "(" + k.over + ") " : "") + "요청 접수함";
  }

  /* ---------- 요청 적기 ---------- */
  function fillSelect(sel, opts, value) {
    var keep = value != null ? value : sel.value;
    sel.replaceChildren.apply(sel, opts.map(function (o) { return h("option", { value: o[0], text: o[1] }); }));
    if (opts.some(function (o) { return o[0] === keep; })) sel.value = keep;
  }
  function progOptions() { return [["", "미정"]].concat(S.programs.map(function (p) { return [p.id, progName(p.id)]; })); }

  /* 요청자가 채워 보낸 양식을 붙여 넣으면 칸이 저절로 나뉩니다. */
  $("ib-paste").addEventListener("input", function () {
    var text = $("ib-paste").value;
    if (!text.trim()) return;
    var f = K.parseAskTemplate(text, S.programs);
    $("ib-what").value = f.what; $("ib-raw").value = f.raw;
    ["why", "when", "check"].forEach(function (k) { $("ib-" + k).value = f[k]; });
    if (f.from) $("ib-from").value = f.from;
    if (f.program) $("ib-prog").value = f.program;
  });

  $("ib-form").addEventListener("submit", function (e) {
    e.preventDefault();
    if (!S.ready) return;
    var names = 0;
    function cl(v) { var r = K.stripNames(v); names += r.n; return r.text; }
    var raw = cl($("ib-raw").value), what = cl($("ib-what").value);
    if (!what) what = K.cleanLine(lines(raw)[0] || "");
    if (!what) { $("ib-what").focus(); toast("무엇(한 줄)을 적어 주세요."); return; }
    if (what.length > 120) what = what.slice(0, 119) + "…";
    var urgent = $("ib-urgent").checked, nowIso = new Date().toISOString();
    var doc = {
      raw: raw, what: what, from: $("ib-from").value.trim(), program: $("ib-prog").value || "", size: $("ib-size").value || "M",
      why: cl($("ib-why").value), when: cl($("ib-when").value), check: cl($("ib-check").value), urgent: urgent,
      status: urgent ? "진행" : "접수", receivedAt: nowIso, askedAt: null, forwardedAt: null, closedAt: null, heldAt: null,
      absorbed: false, absorbedAt: null, est: null, note: "", createdAt: nowIso
    };
    add("requests", doc).then(function (d) {
      if (!d) return;
      ["ib-paste", "ib-what", "ib-raw", "ib-why", "ib-when", "ib-check"].forEach(function (id) { $(id).value = ""; });
      $("ib-urgent").checked = false; $("ib-size").value = "M"; $("ib-prog").value = "";
      var miss = K.missing(doc);
      S.filter = "open"; S.open = null;
      render();
      toast((names ? "이름 " + names + "곳을 [팀원]으로 바꿨습니다. " : "") +
        (urgent ? "긴급 요청으로 기록했습니다." : miss.length ? "적었습니다. 빈칸 " + miss.length + "개를 되묻습니다." : "적었습니다. 빈칸이 없으니 리드에게 넘기면 됩니다."));
    });
  });

  /* ---------- 요청 창구 ---------- */
  /* 요청자에게 줄 링크의 앞부분. 창구가 닫혀 있으면 "". */
  function doorBase() {
    if (!S.ask.on) return "";
    var u = String(S.settings.askUrl || "").trim();
    return u || S.ask.urls[0] || "";
  }
  function newCode() {
    var b = new Uint8Array(10);
    try { window.crypto.getRandomValues(b); } catch (e) { for (var i = 0; i < 10; i++) b[i] = Math.floor(Math.random() * 256); }
    return K.askCodeFrom(b);
  }
  /* 요청에 요청자 링크 코드가 없으면 만들어 붙입니다(직접 적은 요청). 복사는 클릭 안에서 먼저 합니다. */
  function codeFor(r, patch) {
    if (K.ASK_CODE_RE.test(String(r.askCode || ""))) return r.askCode;
    patch.askCode = newCode();
    return patch.askCode;
  }
  function renderDoor() {
    var body = $("ib-door-body"), base = doorBase();
    var tpl = h("button", { cls: "btn sm", type: "button", text: "양식 복사", onclick: function () { copyText(K.templateText(), "양식을 복사했습니다. 요청자에게 보내고, 채워 온 글은 '직접 적기'에 붙여 넣으세요."); } });
    if (base) {
      var sel = null;
      if (S.ask.urls.length > 1 && !String(S.settings.askUrl || "").trim()) {
        sel = h("select", { "aria-label": "이 PC 주소 고르기" });
        fillSelect(sel, S.ask.urls.map(function (u) { return [u, u]; }), base);
        sel.addEventListener("change", function () { putSettings({ askUrl: sel.value }).then(function () { toast("창구 주소를 저장했습니다."); }); });
      }
      var open = h("a", { cls: "btn sm", href: base, target: "_blank", rel: "noopener", text: "요청자 화면 열어 보기" });
      /* 웹 체험판은 요청자 화면이 같은 페이지에 있어 그쪽으로 바꿉니다(NBD_openDoor). */
      open.addEventListener("click", function (e) { if (typeof window.NBD_openDoor === "function") { e.preventDefault(); window.NBD_openDoor(); } });
      body.replaceChildren.apply(body, [
        h("p", { text: "요청자에게 이 링크를 주세요. 요청자가 7칸을 직접 적어 보내면 아래 '받은 요청'에 바로 들어옵니다." }),
        h("div", { cls: "ib-link row" }, h("span", { cls: "mono", text: base }),
          h("button", { cls: "btn sm primary", type: "button", text: "링크 안내 문장 복사", onclick: function () { copyText(K.inviteText(base), "링크 안내 문장을 복사했습니다. 메신저에 붙여 넣으세요."); } }),
          open),
        sel ? h("label", { cls: "ib-pick" }, h("span", { cls: "ex", text: "이 PC 주소가 여러 개입니다. 동료가 들어올 수 있는 주소를 고르세요(보통 10. 이나 192.168. 로 시작)." }), sel) : null,
        h("p", { cls: "ex", text: "메신저·메일로 요청이 오면 옮겨 적지 말고 [링크 안내 문장 복사]로 답하세요. 되물을 때도 빈칸을 채울 링크가 함께 갑니다." }),
        h("div", { cls: "row" }, tpl, h("span", { cls: "ex", text: "링크를 못 여는 분에게는 양식을 보내고, 채워 온 글을 '직접 적기'에 붙여 넣으세요." }))].filter(Boolean));
      return;
    }
    if (S.ask.on) {
      body.replaceChildren(
        h("div", { cls: "callout warn" }, h("p", { text: "창구는 열렸지만(" + (S.ask.port || "?") + " 포트) 이 PC의 주소를 모릅니다. 아래 '설정 · 요청 창구 주소'에 http://PC주소:" + (S.ask.port || 8180) + "/ 을 적어 주세요." })),
        h("div", { cls: "row" }, tpl));
      return;
    }
    body.replaceChildren(
      h("p", { text: "요청 창구가 닫혀 있습니다. 창구를 열면 같은 망의 동료가 링크로 요청을 직접 적어 보내고, 나는 옮겨 적을 일이 없습니다." }),
      h("p", { cls: "ex", text: "여는 법: run-intake.cmd 로 실행하면 함께 열립니다. 실행 파일에 --share 를 붙여도 됩니다. Docker는 -e NOTEBOOK_ASK_ADDR=:8180 -p 8180:8180 을 더합니다. 수첩 화면은 그대로 이 PC에서만 열립니다." }),
      h("div", { cls: "row" }, tpl, h("span", { cls: "ex", text: "창구를 열 수 없으면 요청자에게 양식을 보내고, 채워 온 글을 '직접 적기'에 붙여 넣으세요." })));
  }

  /* ---------- 받은 요청 ---------- */
  ["open", "all", "done"].forEach(function (f) { $("ib-f-" + f).addEventListener("click", function () { S.filter = f; S.open = null; render(); }); });

  function clockPill(r, k) {
    if (k) return pill(k.state === "over" ? "crit" : k.state === "soon" ? "warn" : "good", k.text);
    if (r.urgent) return pill("on", "긴급 · 시계 없음");
    if (r.forwardedAt) return pill("info", "리드에게 넘김 " + fmtDay(r.forwardedAt));
    if (!K.isOpenReq(r)) return pill("info", r.status || "끝남");
    return pill("info", r.status === "보류" ? "보류 · 시계 멈춤" : "시계 멈춤");
  }
  function twoStep(label, run, cls) {
    var b = h("button", { cls: "btn sm " + (cls || ""), type: "button", text: label });
    var timer = null;
    b.addEventListener("click", function () {
      if (b.classList.contains("armed")) { clearTimeout(timer); b.classList.remove("armed"); b.textContent = label; run(); return; }
      b.classList.add("armed"); b.textContent = "한 번 더 누르면 " + label;
      timer = setTimeout(function () { b.classList.remove("armed"); b.textContent = label; }, 4000);
    });
    return b;
  }
  function askNow(r, pn) {
    var p = { askedAt: new Date().toISOString() }, base = doorBase();
    var link = base ? K.askLink(base, codeFor(r, p)) : "";
    copyText(K.askText(r, pn, link), link ? "되묻기 문장을 복사했습니다. 빈칸을 채울 링크가 들어 있습니다." : "되묻기 문장을 복사했습니다. 메신저에 붙여 넣으세요.");
    if ((r.status || "접수") === "접수") p.status = "정리 중";
    upd("requests", r.id, p);
  }
  function statusLinkNow(r) {
    var p = {}, link = K.askLink(doorBase(), codeFor(r, p));
    copyText(K.statusLinkText(r, link), "진행 링크 문장을 복사했습니다.");
    if (p.askCode) upd("requests", r.id, p);
  }
  function handNow(r, pn) {
    copyText(K.handoffText(r, pn), "넘길 문장을 복사했습니다.");
    var st = r.status || "접수";
    if (st === "접수" || st === "정리 중") {
      upd("requests", r.id, K.statusPatch(r, "리드 전달", new Date().toISOString())).then(function () {
        toast("넘길 문장을 복사하고 '리드 전달'로 바꿨습니다. 전달 시계가 멈춥니다.");
      });
    }
  }
  function field(r, key, label, el) { return h("label", { cls: key === "raw" || key === "what" ? "wide" : null }, h("span", { text: label }), el); }
  function input(r, key, ph) {
    var el = h("input", { type: "text", autocomplete: "off", placeholder: ph || "", "data-key": key }); el.value = r[key] || "";
    el.addEventListener("change", function () { var p = {}; p[key] = K.stripNames(el.value).text; upd("requests", r.id, p); });
    return el;
  }
  function editor(r, pn) {
    var raw = h("textarea", { rows: 3 }); raw.value = r.raw || "";
    raw.addEventListener("change", function () { upd("requests", r.id, { raw: K.stripNames(raw.value).text }); });
    var from = h("input", { type: "text", autocomplete: "off", list: "ib-roles-list", placeholder: "이름이나 역할" }); from.value = r.from || "";
    from.addEventListener("change", function () { upd("requests", r.id, { from: from.value.trim() }); });
    var prog = h("select"); fillSelect(prog, progOptions(), r.program || "");
    prog.addEventListener("change", function () { upd("requests", r.id, { program: prog.value }); });
    var size = h("select"); fillSelect(size, [["S", "S 반나절 이하"], ["M", "M 1~5일"], ["L", "L 사이클급"]], r.size || "M");
    size.addEventListener("change", function () { upd("requests", r.id, { size: size.value }); });
    var recv = h("input", { type: "datetime-local" }); recv.value = localInput(r.receivedAt);
    recv.addEventListener("change", function () { var iso = fromLocal(recv.value); if (iso) upd("requests", r.id, { receivedAt: iso }); });
    var urgent = h("input", { type: "checkbox" }); urgent.checked = !!r.urgent;
    urgent.addEventListener("change", function () { upd("requests", r.id, { urgent: urgent.checked }); });
    var form = h("div", { cls: "form" },
      field(r, "what", "무엇(한 줄)", input(r, "what")),
      field(r, "raw", r.via === "창구" ? "자세히(요청자가 적은 것)" : "자세히", raw),
      field(r, "from", "보낸 분", from),
      field(r, "program", "어느 프로그램", prog),
      field(r, "size", "크기", size),
      field(r, "why", "왜·누가 결정", input(r, "why", "모르면 비워 둡니다")),
      field(r, "when", "언제까지와 이유", input(r, "when")),
      field(r, "check", "확인 방법", input(r, "check")),
      field(r, "recv", "받은 때", recv),
      h("label", { cls: "check" }, urgent, h("span", { text: "긴급(리드가 바로 처리)" })));
    var miss = K.missing(r), base = doorBase(), link = base && r.askCode ? K.askLink(base, r.askCode) : "";
    return h("div", { cls: "ib-edit" }, form,
      h("p", { cls: "label", text: miss.length ? "되묻기 문장(빈칸만)" : "되물을 빈칸이 없습니다" }),
      miss.length ? h("div", { cls: "say", text: K.askText(r, pn, link) }) : null,
      base ? h("div", { cls: "row" }, h("span", { cls: "label", text: "요청자 진행 링크" }), link ? h("span", { cls: "mono", text: link }) : null,
        h("button", { cls: "btn sm", type: "button", text: "진행 링크 문장 복사", onclick: function () { statusLinkNow(r); } })) : null,
      h("p", { cls: "label", text: "리드에게 넘길 문장" }), h("div", { cls: "say", text: K.handoffText(r, pn) }),
      h("div", { cls: "row between" },
        h("span", { cls: "ex tnum", text: [r.askedAt ? "되물음 " + fmtDT(r.askedAt) : "", r.forwardedAt ? "넘김 " + fmtDT(r.forwardedAt) : "", r.closedAt ? "닫음 " + fmtDT(r.closedAt) : ""].filter(Boolean).join(" · ") }),
        twoStep("지우기", function () { S.open = null; del("requests", r.id).then(function () { toast("요청을 지웠습니다."); }); }, "danger")));
  }
  function item(r, now, c) {
    var k = K.reqClock(r, now, c), miss = K.missing(r), open = K.isOpenReq(r), pn = progName(r.program), editing = S.open === r.id;
    var st = r.status || "접수", early = st === "접수" || st === "정리 중";
    var status = h("select", { "aria-label": "상태" }); fillSelect(status, STATUSES.map(function (s) { return [s, s]; }), st);
    status.addEventListener("change", function () {
      var p = K.statusPatch(r, status.value, new Date().toISOString());
      upd("requests", r.id, p).then(function () { if (p.forwardedAt) toast("리드에게 넘긴 것으로 적었습니다. 전달 시계가 멈춥니다."); });
    });
    var askB = open && early && miss.length && !r.urgent
      ? h("button", { cls: "btn sm primary", type: "button", text: "되묻기 문장 복사", onclick: function () { askNow(r, pn); } }) : null;
    var handB = open && (early || r.urgent)
      ? h("button", { cls: "btn sm" + (askB ? "" : " primary"), type: "button", text: early ? "넘길 문장 복사 · 리드 전달로" : "넘길 문장 복사", onclick: function () { if (early) handNow(r, pn); else copyText(K.handoffText(r, pn), "넘길 문장을 복사했습니다."); } }) : null;
    var editB = h("button", { cls: "btn sm quiet", type: "button", "aria-expanded": editing ? "true" : "false", text: editing ? "고치기 닫기" : "고치기", onclick: function () { S.open = editing ? null : r.id; render(); } });
    return h("li", { cls: "ib-item " + (k ? k.state : "") + (open ? "" : " closed"), "data-id": r.id },
      h("div", { cls: "ib-row1" }, clockPill(r, k), h("span", { cls: "ib-what", text: K.reqTitle(r) }), r.urgent ? pill("on", "긴급") : null,
        r.via === "창구" ? pill("on", "창구") : null,
        r.addedAt && early && (!r.askedAt || r.addedAt > r.askedAt) ? pill("good", "요청자가 보탬 " + fmtDay(r.addedAt)) : r.askedAt && early ? pill("info", "되물음 " + fmtDay(r.askedAt)) : null),
      h("div", { cls: "ib-meta ex tnum", text: [r.from || "보낸 분 미정", pn || "프로그램 미정", r.size || "", "받음 " + fmtDT(r.receivedAt)].filter(Boolean).join(" · ") }),
      r.raw && r.raw.trim() !== K.reqTitle(r) && !editing ? h("div", { cls: "ib-raw", text: r.raw }) : null,
      open && !r.urgent ? h("div", { cls: "ib-gaps" }, miss.length ? [h("span", { cls: "ex", text: "빈칸" })].concat(miss.map(function (m) { return pill("warn", m); })) : pill("good", "빈칸 없음")) : null,
      h("div", { cls: "ib-actions" }, status, askB, handB, editB),
      editing ? editor(r, pn) : null);
  }
  function renderList(now) {
    ["open", "all", "done"].forEach(function (f) { $("ib-f-" + f).setAttribute("aria-pressed", S.filter === f ? "true" : "false"); });
    var c = calNow(), list = K.intakeView(S.requests, S.filter, now, c), ul = $("ib-list");
    ul.replaceChildren.apply(ul, list.map(function (r) { return item(r, now, c); }));
    if (!list.length) ul.append(h("li", { cls: "empty", text: S.requests.length ? "이 보기에는 요청이 없습니다." : doorBase() ? "아직 받은 요청이 없습니다. 위 링크를 요청자에게 주면, 요청자가 적어 보낸 것이 여기 바로 들어옵니다." : "아직 받은 요청이 없습니다. 요청 창구를 열거나, 말로 받은 요청은 '직접 적기'로 적으세요." }));
  }

  /* ---------- 설정 ---------- */
  function renderPrograms() {
    var ul = $("ib-progs");
    var used = {}; S.requests.forEach(function (r) { if (r.program) used[r.program] = (used[r.program] || 0) + 1; });
    ul.replaceChildren.apply(ul, S.programs.map(function (p) {
      var name = h("input", { type: "text", autocomplete: "off", "aria-label": "프로그램 이름" }); name.value = p.name || "";
      name.addEventListener("change", function () { upd("programs", p.id, { name: name.value.trim() }); });
      var n = used[p.id] || 0;
      return h("li", null, name, n ? h("span", { cls: "ex", text: "요청 " + n + "건" }) : twoStep("지우기", function () { del("programs", p.id); }, "danger"));
    }));
    if (!S.programs.length) ul.append(h("li", { cls: "empty", text: "아직 없습니다. 아래에 이름을 적고 [추가]를 누르세요." }));
  }
  $("ib-prog-form").addEventListener("submit", function (e) {
    e.preventDefault();
    var name = $("ib-prog-new").value.trim();
    if (!name) return;
    var max = S.programs.reduce(function (m, p) { var o = Number(p.order); return isFinite(o) && o > m ? o : m; }, 0);
    add("programs", { order: max + 1, name: name, system: "", lead: "", sub: "", active: true, subImpl: false, brief: { problem: "", success: "", constraints: "", out: "" }, note: "" })
      .then(function (d) { if (d) { $("ib-prog-new").value = ""; toast("프로그램을 추가했습니다."); } });
  });
  $("ib-askurl").addEventListener("change", function () {
    var v = $("ib-askurl").value.trim();
    if (v && !/^https?:\/\/[^\s\/]+/i.test(v)) { toast("http:// 로 시작하는 주소를 적어 주세요. 예: http://10.0.0.5:8180/"); return; }
    putSettings({ askUrl: v }).then(function () { toast(v ? "창구 주소를 저장했습니다." : "창구 주소를 자동으로 되돌렸습니다."); });
  });
  $("ib-roles").addEventListener("change", function () { var r = lines($("ib-roles").value); putSettings({ requesterRoles: r.length ? r : K.REQUESTER_ROLES.slice() }).then(function () { toast("요청자 역할을 저장했습니다."); }); });
  $("ib-hol").addEventListener("change", function () { putSettings({ holidays: $("ib-hol").value }).then(function () { toast("쉬는 날을 저장했습니다. 근무일 시계에 바로 반영됩니다."); }); });
  (function () {
    var b = $("ib-import"), armed = false, timer = null;
    b.addEventListener("click", function () {
      var f = $("ib-import-file").files && $("ib-import-file").files[0];
      if (!f) { toast("먼저 가져올 .json 파일을 고르세요."); return; }
      if (!armed) { armed = true; b.classList.add("armed"); b.textContent = "한 번 더 누르면 가져옵니다"; timer = setTimeout(function () { armed = false; b.classList.remove("armed"); b.textContent = "가져오기"; }, 5000); return; }
      clearTimeout(timer); armed = false; b.classList.remove("armed"); b.textContent = "가져오기";
      f.text().then(function (txt) {
        return write(api("POST", "/api/import", txt)).then(function (res) {
          if (res && res.replaced) toast("가져왔습니다: " + (res.replaced.length ? res.replaced.join(", ") : "바꾼 항목 없음") + (res.settings ? " · 설정" : ""));
        });
      }, function () { toast("파일을 읽지 못했습니다."); });
    });
  })();
  function renderSettings() {
    if (document.activeElement !== $("ib-roles")) $("ib-roles").value = roles().join("\n");
    if (document.activeElement !== $("ib-hol")) $("ib-hol").value = S.settings.holidays || "";
    if (document.activeElement !== $("ib-askurl")) { $("ib-askurl").value = S.settings.askUrl || ""; $("ib-askurl").placeholder = S.ask.urls[0] ? "자동: " + S.ask.urls[0] : "자동"; }
    renderPrograms();
    $("ib-full").hidden = S.only === "intake";
  }

  /* ---------- 그리기 ---------- */
  function render() {
    var now = new Date();
    $("demo-band").hidden = !S.demo;
    renderCounts(now);
    renderDoor();
    var dl = $("ib-roles-list"); dl.replaceChildren.apply(dl, roles().map(function (x) { return h("option", { value: x }); }));
    fillSelect($("ib-prog"), progOptions());
    renderList(now);
    renderSettings();
    var foot = [h("span", { text: "요청 접수함 · 관리 수첩 v" + (S.version || "?") + "의 한 기능판 · 외부 연결 없음 · 기록은 이 PC 안에만 저장" })];
    if (S.auth) foot.push(h("span", null, " · ", h("button", { cls: "btn quiet sm", type: "button", text: "로그아웃", onclick: function () { api("POST", "/api/logout").then(goLogin, goLogin); } })));
    $("ib-foot").replaceChildren.apply($("ib-foot"), foot);
  }
  /* 다른 탭에 있다가 돌아오면 바로 새로 읽습니다. */
  document.addEventListener("visibilitychange", function () { if (!document.hidden && S.loaded && !S.open) refresh(); });
  /* 창구로 들어온 요청과 근무일 시계를 30초마다 새로 읽어 그립니다. 고치거나 적는 중에는 건드리지 않습니다. */
  setInterval(function () {
    if (!S.loaded || S.open || document.hidden) return;
    var a = document.activeElement;
    if (a && /^(INPUT|TEXTAREA|SELECT)$/.test(a.tagName)) return;
    refresh();
  }, 30000);

  if (!K) { storeNote("화면 파일(core.js)을 읽지 못했습니다. 새로 고침(Ctrl+F5)해 주세요."); return; }
  setReady(false);
  refresh();
})();
