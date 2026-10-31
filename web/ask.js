/* 요청 창구 — 요청자가 직접 적어 보내는 화면.
 * 새 요청 보내기(POST /api/ask), 내 요청 보기(GET /api/ask/{code}), 빈칸 채우기·덧붙이기(PATCH /api/ask/{code}),
 * 취소(POST /api/ask/{code}/withdraw), 끝남 확인(POST /api/ask/{code}/done)만 합니다.
 * '지금 누구 차례인지'와 날짜는 core.js(NBCore.askNow)가 계산합니다. 수첩의 다른 화면·API는 이 포트에 없습니다.
 * CSP: 인라인 스크립트·style 속성을 쓰지 않습니다.
 */
(function () {
  "use strict";
  var K = window.NBCore;
  var SENT_KEY = "mn-ask:sent", FROM_KEY = "mn-ask:from";
  var FIELDS = {
    why: ["왜 필요한가요? 누가 정한 일인가요?", "예: 다음 달 시연용, 팀장 지시"],
    when: ["언제까지 필요한가요? 그 날짜인 이유는?", "예: 11/20까지, 리허설이 11/21"],
    check: ["끝났다는 것을 어떻게 확인하면 될까요?", "예: 버튼을 누르면 파일이 받아짐"]
  };
  var ADD_KIND = { cancel: "취소하며", notyet: "아직 다름" };
  var S = { programs: [], version: "", demo: false, busy: false, code: "", cal: null, view: null };

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
      else if (v !== undefined && v !== null && v !== false) n.setAttribute(k, v === true ? "" : v);
    }
    var kids = Array.prototype.slice.call(arguments, 2);
    (function put(list) { list.forEach(function (c) { if (c == null || c === false) return; if (Array.isArray(c)) put(c); else n.append(c.nodeType ? c : String(c)); }); })(kids);
    return n;
  }
  function pad2(n) { return String(n).padStart(2, "0"); }
  /* 요청자 자신의 일(보낸 때·덧붙인 때)은 시각까지, 담당자 쪽 일은 날짜만 보입니다. */
  function fmtDT(x) { var d = x ? new Date(x) : null; return d && !isNaN(d) ? (d.getMonth() + 1) + "/" + d.getDate() + " (" + K.WD[d.getDay()] + ") " + pad2(d.getHours()) + ":" + pad2(d.getMinutes()) : ""; }
  function clean(v) { return K.stripNames(v).text; }
  function load(key, dflt) { try { var v = localStorage.getItem(key); return v == null ? dflt : JSON.parse(v); } catch (e) { return dflt; } }
  function save(key, v) { try { localStorage.setItem(key, JSON.stringify(v)); } catch (e) { /* 저장이 막힌 브라우저: 목록만 못 남김 */ } }
  /* NBD_DOOR: 웹 체험판이 보여 줄 예시 창구 주소(실제 창구에서는 없음) */
  function linkOf(code) { return (typeof window.NBD_DOOR === "string" && window.NBD_DOOR ? window.NBD_DOOR.replace(/\/+$/, "") : location.origin) + "/r/" + code; }

  var toastTimer = null;
  function toast(msg) { var el = $("toast"); el.textContent = msg; el.hidden = false; clearTimeout(toastTimer); toastTimer = setTimeout(function () { el.hidden = true; }, 3600); }
  function note(msg) { var n = $("ask-note"); n.hidden = !msg; n.textContent = msg || ""; }

  function api(method, path, body) {
    var init = { method: method, headers: { "Accept": "application/json" }, credentials: "same-origin", cache: "no-store" };
    if (body !== undefined) { init.headers["Content-Type"] = "application/json"; init.body = JSON.stringify(body); }
    return fetch(path, init).then(function (res) {
      return res.text().then(function (txt) {
        var data = null;
        if (txt) { try { data = JSON.parse(txt); } catch (e) { data = null; } }
        if (!res.ok) throw { status: res.status, code: (data && data.error) || ("http_" + res.status), message: (data && data.message) || "" };
        return data;
      });
    }, function () { throw { code: "network", message: "담당자 PC에 연결하지 못했습니다. 담당자 PC가 켜져 있는 근무 시간에 다시 해 주세요." }; });
  }
  function errMsg(e, dflt) { return e && e.message ? e.message : dflt; }

  /* ---------- 복사 ---------- */
  function copyText(text, done) {
    var ok = function () { toast(done || "복사했습니다."); };
    var fail = function () {
      var ta = h("textarea", { cls: "offscreen", "aria-hidden": "true", readonly: true }); ta.value = text;
      document.body.append(ta); ta.select();
      var good = false; try { good = document.execCommand("copy"); } catch (e) { good = false; }
      ta.remove();
      if (good) ok(); else window.prompt("Ctrl+C로 복사하세요.", text);
    };
    try {
      if (navigator.clipboard && navigator.clipboard.writeText) navigator.clipboard.writeText(text).then(ok, fail);
      else fail();
    } catch (e) { fail(); }
  }

  /* ---------- 새 요청 ---------- */
  function progOptions(sel, value) {
    sel.replaceChildren(h("option", { value: "", text: "잘 모르겠음" }));
    S.programs.forEach(function (p) { sel.append(h("option", { value: p.id, text: p.name })); });
    if (value && S.programs.some(function (p) { return p.id === value; })) sel.value = value;
  }

  $("ask-form").addEventListener("submit", function (e) {
    e.preventDefault();
    if (S.busy) return;
    var what = clean($("ask-what").value), from = $("ask-from").value.trim();
    if (!what) { $("ask-what").focus(); toast("무엇이 필요한지 한 줄은 적어 주세요."); return; }
    if (!from) { $("ask-from").focus(); toast("보내는 분을 적어 주세요. 되물을 때 필요합니다."); return; }
    var body = {
      what: what, from: from, raw: clean($("ask-raw").value),
      why: clean($("ask-why").value), when: clean($("ask-when").value), check: clean($("ask-check").value),
      program: $("ask-prog").value || ""
    };
    S.busy = true; $("ask-send").disabled = true;
    api("POST", "/api/ask", body).then(function (v) {
      save(FROM_KEY, from);
      ["ask-what", "ask-raw", "ask-why", "ask-when", "ask-check"].forEach(function (id) { $(id).value = ""; });
      $("ask-prog").value = "";
      note("");
      go(v.code, v, true);
    }, function (err) {
      toast(errMsg(err, "보내지 못했습니다. 잠시 뒤 다시 해 주세요."));
    }).then(function () { S.busy = false; $("ask-send").disabled = false; });
  });

  /* ---------- 내 요청 보기 ---------- */
  function steps(v, now) {
    if (now.step < 0) return null;
    return h("ol", { cls: "ask-steps", "aria-label": "진행 단계" }, K.ASK_STEPS.map(function (label, i) {
      return h("li", { cls: i < now.step ? "done" : i === now.step ? "now" : "", "aria-current": i === now.step ? "step" : null, text: label });
    }));
  }
  function row(label, value) { return value ? [h("dt", { text: label }), h("dd", { text: value })] : null; }
  /* 보내기 버튼 하나를 가진 작은 양식. send(body)는 Promise. */
  function sender(btn, run) {
    return function (e) {
      if (e) e.preventDefault();
      if (S.busy) return;
      var p = run();
      if (!p) return;
      S.busy = true; btn.disabled = true;
      p.then(function (nv) { if (nv) render(nv); }, function (err) {
        toast(errMsg(err, "보내지 못했습니다. 잠시 뒤 다시 해 주세요."));
        if (err && (err.code === "locked" || err.code === "withdrawn" || err.code === "not_yet" || err.code === "closed")) show(S.code);
      }).then(function () { S.busy = false; btn.disabled = false; });
    };
  }
  /* 빈칸 채우기(리드에게 넘어가기 전) + 덧붙임(언제나, 취소 전까지) */
  function writeForm(v) {
    var gaps = v.editable ? (Array.isArray(v.missing) ? v.missing : []).filter(function (k) { return FIELDS[k]; }) : [];
    if (!gaps.length && !v.canAdd) return null;
    var inputs = {};
    var kids = gaps.map(function (k) {
      var el = h("input", { type: "text", id: "ask-f-" + k, maxlength: "500", autocomplete: "off", placeholder: FIELDS[k][1] });
      inputs[k] = el;
      return h("label", { "for": "ask-f-" + k }, h("span", { text: FIELDS[k][0] }), el);
    });
    var prog = null;
    if (v.editable && !v.program && S.programs.length) {
      prog = h("select", { id: "ask-f-program" }); progOptions(prog, "");
      kids.push(h("label", { "for": "ask-f-program" }, h("span", { text: "어느 프로그램인지 아시면 골라 주세요(선택)" }), prog));
    }
    var st = v.status, fwd = !!v.forwardedAt, closed = st === "완료" || st === "종료";
    var moreHint = closed ? "다시 봐 주셨으면 하는 것이 있으면 적어 주세요. 담당자 접수함에 다시 올라갑니다."
      : fwd ? "넘긴 뒤 생긴 일(날짜·범위가 바뀜 등)을 적어 주세요. 담당자가 읽고 담당 리드에게 전합니다."
      : "더 알려 주실 것이 있으면 적어 주세요.";
    var moreEx = closed ? "예: 받은 파일 첫 줄에 머리글이 없습니다" : fwd ? "예: 리허설이 11/18로 당겨졌습니다" : "예: 참고할 문서는 공유 폴더 ○○에 있습니다";
    var more = v.canAdd ? h("textarea", { id: "ask-f-more", rows: "2", maxlength: "1000", placeholder: moreEx }) : null;
    var send = h("button", { cls: "btn primary", type: "submit", text: gaps.length ? "채운 내용 보내기" : "덧붙이기" });
    var form = h("form", { cls: "form ask-fill", novalidate: true },
      gaps.length ? h("p", { cls: "wide label", text: "빈칸 " + gaps.length + "개를 채워 주세요" }) : null,
      kids, more ? h("label", { cls: "wide", "for": "ask-f-more" }, h("span", { text: gaps.length ? "덧붙임(선택)" : moreHint }), more) : null,
      h("div", { cls: "wide row" }, send, gaps.length ? h("span", { cls: "ex", text: st === "보류" ? "채워 주시면 담당자가 확인해 다시 이어 갑니다." : "채워 주시면 담당자가 확인해 근무일 2일 안에 담당 리드에게 넘깁니다." }) : null));
    form.addEventListener("submit", sender(send, function () {
      var body = {}, any = false;
      Object.keys(inputs).forEach(function (k) { var x = clean(inputs[k].value); if (x) { body[k] = x; any = true; } });
      if (prog && prog.value) { body.program = prog.value; any = true; }
      var m = more ? clean(more.value) : "";
      if (m) { body.more = m; any = true; }
      if (!any) { toast("채운 칸이 없습니다."); return null; }
      return api("PATCH", "/api/ask/" + v.code, body).then(function (nv) { toast("보냈습니다. 담당자 접수함에 바로 보입니다."); return nv; });
    }));
    return form;
  }
  /* 끝남 확인: 리드에게 넘어간 뒤에만. 끝남 단계에서는 '아직 다릅니다'도 받습니다. */
  function doneBox(v, now) {
    if (!v.canConfirm || now.who === "") return null;
    var done = v.status === "완료";
    if (!done && K.doneState(v) === "ok") return null; /* 이미 끝났다고 알려 주심 */
    var why = h("textarea", { rows: "2", maxlength: "1000", "aria-label": "무엇이 다른지", placeholder: "무엇이 다른지 한 줄 적어 주세요" });
    var noForm = h("div", { cls: "stack", hidden: true }, why);
    var okB = h("button", { cls: "btn sm primary", type: "button", text: done ? "확인했습니다" : "결과를 받았습니다 · 끝났어요" });
    var noB = h("button", { cls: "btn sm", type: "button", text: "아직 다릅니다" });
    var noSend = h("button", { cls: "btn sm", type: "button", text: "보내기" });
    okB.addEventListener("click", sender(okB, function () {
      return api("POST", "/api/ask/" + v.code + "/done", { ok: true }).then(function (nv) { toast("고맙습니다. 담당자에게 끝났다고 전했습니다."); return nv; });
    }));
    noB.addEventListener("click", function () { noForm.hidden = false; why.focus(); });
    noSend.addEventListener("click", sender(noSend, function () {
      var t = clean(why.value);
      if (!t) { why.focus(); toast("무엇이 다른지 한 줄 적어 주세요."); return null; }
      return api("POST", "/api/ask/" + v.code + "/done", { ok: false, text: t }).then(function (nv) { toast("보냈습니다. 담당자 접수함에 바로 보입니다."); return nv; });
    }));
    noForm.append(h("div", { cls: "row" }, noSend));
    return h("div", { cls: "ask-done stack" },
      h("p", { cls: "label", text: done ? "확인해 보셨나요?" : "결과를 이미 받으셨나요?" }),
      h("div", { cls: "row" }, okB, done ? noB : null),
      done ? noForm : null);
  }
  /* 요청 취소(두 번 눌러 확인). 넘긴 뒤라면 담당자가 리드에게 알리고 닫습니다. */
  function cancelBox(v) {
    if (!v.canWithdraw) return null;
    var why = h("textarea", { rows: "2", maxlength: "1000", "aria-label": "취소하는 까닭", placeholder: "까닭(선택). 예: 다른 방법으로 해결했습니다" });
    var go1 = h("button", { cls: "btn sm danger", type: "button", text: "요청 취소하기" });
    var back = h("button", { cls: "btn sm quiet", type: "button", text: "그만두기" });
    var more = h("div", { cls: "stack", hidden: true }, why, h("div", { cls: "row" }, go1, back),
      h("p", { cls: "ex", text: K.leadHas(v) ? "이미 담당 리드가 맡은 요청입니다. 취소하시면 담당자가 리드에게 알리고 닫습니다." : "취소하면 되돌릴 수 없습니다. 다시 필요하면 새로 보내 주세요." }));
    var open = h("button", { cls: "btn sm quiet", type: "button", text: "이 요청이 더 필요 없으신가요? 요청 취소" });
    open.addEventListener("click", function () { more.hidden = false; open.hidden = true; why.focus(); });
    back.addEventListener("click", function () { more.hidden = true; open.hidden = false; });
    go1.addEventListener("click", sender(go1, function () {
      var body = {}, t = clean(why.value);
      if (t) body.text = t;
      return api("POST", "/api/ask/" + v.code + "/withdraw", body).then(function (nv) { toast("취소했습니다."); return nv; });
    }));
    return h("div", { cls: "ask-cancel" }, open, more);
  }
  function addsList(v) {
    var list = Array.isArray(v.adds) ? v.adds.filter(function (a) { return a && a.text; }) : [];
    if (!list.length) return null;
    return h("div", { cls: "ask-adds" }, h("p", { cls: "label", text: "덧붙인 말" }),
      h("ul", null, list.map(function (a) {
        return h("li", null, h("span", { cls: "ex tnum", text: fmtDT(a.at) + (ADD_KIND[a.kind] ? " · " + ADD_KIND[a.kind] : "") }), h("div", { text: a.text }));
      })));
  }
  function rulesList() { return h("ol", { cls: "ask-rule-list" }, K.ASK_RULES.map(function (r) { return h("li", { text: r }); })); }
  function render(v, fresh) {
    S.view = v;
    var now = K.askNow(v, new Date(), S.cal), link = linkOf(v.code);
    var box = $("ask-view");
    box.replaceChildren.apply(box, [
      fresh ? h("div", { cls: "callout good" }, h("b", { text: "보냈습니다." }), h("span", { text: " 아래 링크를 저장해 두세요. 진행 상황과 날짜가 이 링크에 보입니다(담당자 PC가 켜져 있을 때 열립니다)." })) : null,
      h("div", { cls: "row between" }, h("h2", { text: v.what || "(제목 없음)" }), h("span", { cls: "pill " + (now.step < 0 ? "warn" : "on"), text: now.label })),
      steps(v, now),
      h("div", { cls: "ask-now who-" + (now.who || "end") },
        now.whoLabel || K.shortDay(now.last) ? h("p", { cls: "ask-who" }, now.whoLabel ? h("b", { text: now.whoLabel }) : null,
          K.shortDay(now.last) ? h("span", { cls: "ex", text: (now.whoLabel ? " · " : "") + "마지막 소식 " + K.shortDay(now.last) }) : null) : null,
        now.lines.map(function (t) { return h("p", { text: t }); })),
      K.replyNow(v) ? h("div", { cls: "ask-reply" }, h("p", { cls: "label", text: "담당자가 남긴 말" + (K.shortDay(v.replyAt) ? " · " + K.shortDay(v.replyAt) : "") }), h("p", { text: K.replyNow(v) })) : null,
      doneBox(v, now),
      writeForm(v),
      h("div", { cls: "ask-link stack" },
        h("div", { cls: "row" }, h("span", { cls: "mono", text: link }),
          h("button", { cls: "btn sm", type: "button", text: "링크 복사", onclick: function () { copyText(link, "링크를 복사했습니다."); } }),
          h("button", { cls: "btn sm", type: "button", text: "보고용 한 줄 복사", onclick: function () { copyText(K.askReportText(v, link, new Date(), S.cal), "보고용 한 줄을 복사했습니다."); } })),
        h("p", { cls: "ex", text: "이 링크를 가진 사람은 누구나 이 요청을 보고, 덧붙이고, 취소할 수 있습니다. 단체방에는 올리지 마세요." })),
      h("dl", { cls: "ask-dl" },
        row("보낸 분", v.from), row("받은 때", fmtDT(v.receivedAt)), row("프로그램", v.programName || "담당자가 고릅니다"),
        row("왜·누가 정했는지", v.why), row("언제까지와 이유", v.when), row("확인 방법", v.check)),
      v.raw ? h("div", { cls: "ask-raw" }, h("p", { cls: "label", text: "자세히" }), h("div", { text: v.raw })) : null,
      addsList(v),
      cancelBox(v),
      h("details", { cls: "more" }, h("summary", { text: "요청은 이렇게 처리됩니다" }), rulesList()),
      h("div", { cls: "row" }, h("button", { cls: "btn", type: "button", text: "새 요청 보내기", onclick: home }))].filter(Boolean));
    box.hidden = false;
    $("ask-new").hidden = true; $("ask-rules").hidden = true;
    document.title = (v.what || "요청") + " · 요청 보내기";
    remember(v);
    renderMine();
  }
  function show(code, fresh) {
    note("");
    return api("GET", "/api/ask/" + code).then(function (v) { render(v, fresh); }, function (err) {
      S.view = null;
      $("ask-view").hidden = true; $("ask-new").hidden = false; $("ask-rules").hidden = false;
      note(errMsg(err, "이 링크의 요청을 찾지 못했습니다."));
    });
  }
  /* 화면 안에서 옮겨 다닙니다(주소도 바꿈). 웹 체험판처럼 주소를 못 바꾸는 곳에서는 화면만 바꿉니다. */
  function setPath(p) { if (window.NBD_ONLY) return; try { if (location.pathname !== p) history.pushState(null, "", p); } catch (e) { /* 주소를 못 바꿔도 화면은 바뀜 */ } }
  function go(code, v, fresh) { S.code = code; setPath("/r/" + code); if (v) render(v, fresh); else show(code); window.scrollTo(0, 0); }
  function home() {
    S.code = ""; S.view = null; setPath("/");
    $("ask-view").hidden = true; $("ask-new").hidden = false; $("ask-rules").hidden = false; note("");
    document.title = "요청 보내기";
    renderMine();
    $("ask-what").focus();
  }
  function route() {
    var m = /^\/r\/([^\/?#]+)\/?$/.exec(location.pathname);
    if (m && !window.NBD_ONLY) { S.code = m[1]; show(m[1]); } else home();
  }
  window.addEventListener("popstate", route);
  /* 다른 탭에 있다가 돌아오면 보고 있던 요청을 새로 읽습니다(무언가 적는 중이면 그대로 둠). */
  document.addEventListener("visibilitychange", function () {
    if (document.hidden || !S.code || S.busy || $("ask-view").hidden) return;
    var a = document.activeElement;
    if (a && $("ask-view").contains(a) && /^(INPUT|TEXTAREA|SELECT)$/.test(a.tagName)) return;
    var typed = Array.prototype.some.call($("ask-view").querySelectorAll("input, textarea"), function (el) { return el.value && el.type !== "button"; });
    if (typed) return;
    show(S.code);
  });
  /* 웹 체험판: 접수함에서 '요청자 화면으로 보기'를 누르면 그 요청을 엽니다. */
  if (window.NBD_ONLY) window.NBD_showAsk = function (code) { if (K.ASK_CODE_RE.test(String(code || ""))) go(code); else home(); };

  /* ---------- 이 브라우저에서 본 요청 ---------- */
  function sentList() { var s = load(SENT_KEY, []); return (Array.isArray(s) ? s : []).filter(function (x) { return x && K.ASK_CODE_RE.test(String(x.code)); }); }
  /* 보낸 요청과 링크로 연 요청을 모두 남깁니다(최근 30개). */
  function remember(v) {
    if (!v || !K.ASK_CODE_RE.test(String(v.code))) return;
    var list = sentList(), i = list.findIndex(function (x) { return x.code === v.code; });
    var item = { code: v.code, what: v.what || "", at: v.receivedAt || "" };
    if (i >= 0) list[i] = item; else list.unshift(item);
    save(SENT_KEY, list.slice(0, 30));
  }
  function renderMine() {
    var sent = sentList();
    $("ask-mine").hidden = !sent.length;
    var ul = $("ask-mine-list");
    ul.replaceChildren.apply(ul, sent.map(function (x) {
      var a = h("a", { href: "/r/" + x.code, text: x.what || "(제목 없음)" });
      a.addEventListener("click", function (e) { if (e.ctrlKey || e.metaKey || e.shiftKey) return; e.preventDefault(); go(x.code); });
      return h("li", { cls: x.code === S.code ? "now" : null }, a, h("span", { cls: "ex tnum", text: fmtDT(x.at) }));
    }));
  }
  $("ask-mine-clear").addEventListener("click", function () { save(SENT_KEY, []); renderMine(); toast("이 브라우저의 목록을 지웠습니다. 요청은 그대로 있습니다."); });

  /* ---------- 시작 ---------- */
  if (!K) { note("화면 파일(core.js)을 읽지 못했습니다. 새로 고침(Ctrl+F5)해 주세요."); return; }
  S.cal = K.cal(K.HOLIDAYS_DEFAULT);
  $("ask-rules-list").replaceChildren.apply($("ask-rules-list"), K.ASK_RULES.map(function (r) { return h("li", { text: r }); }));
  var f = load(FROM_KEY, "");
  if (typeof f === "string") $("ask-from").value = f;
  api("GET", "/api/ask/form").then(function (d) {
    d = d || {};
    S.programs = Array.isArray(d.programs) ? d.programs : [];
    S.version = d.version ? String(d.version) : "";
    S.demo = !!d.demo;
    /* 쉬는 날을 적은 적이 없으면(null) 수첩과 같은 기본 공휴일을 씁니다. */
    S.cal = K.cal(typeof d.holidays === "string" ? d.holidays : K.HOLIDAYS_DEFAULT);
    progOptions($("ask-prog"), $("ask-prog").value);
    $("ask-demo").hidden = !S.demo;
    var owner = typeof d.owner === "string" ? d.owner.trim() : "";
    $("ask-owner").hidden = !owner; $("ask-owner").textContent = owner ? "받는 사람: " + owner : "";
    var notice = typeof d.notice === "string" ? d.notice.trim() : "";
    $("ask-notice").hidden = !notice; $("ask-notice").textContent = notice;
    $("ask-foot").textContent = "요청 창구" + (S.version ? " · v" + S.version : "");
  }, function (err) { note(errMsg(err, "담당자 PC에 연결하지 못했습니다.")); }).then(route);
})();
