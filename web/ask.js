/* 요청 창구 — 요청자가 직접 적어 보내는 화면.
 * 새 요청 보내기(POST /api/ask), 내 요청 보기(GET /api/ask/{code}), 빈칸 채우기·덧붙이기(PATCH /api/ask/{code})만 합니다.
 * 이 포트에는 관리 수첩의 다른 화면·API가 없습니다. CSP: 인라인 스크립트·style 속성을 쓰지 않습니다.
 */
(function () {
  "use strict";
  var K = window.NBCore;
  var SENT_KEY = "mn-ask:sent", FROM_KEY = "mn-ask:from";
  var FIELDS = {
    why: ["왜 필요한가요? 누가 정한 일인가요?", "예: 다음 달 시연용, 팀장 지시"],
    when: ["언제까지 필요한가요? 그 날짜인 이유는?", "예: 11/20까지, 리허설이 11/21"],
    check: ["끝났다는 것을 어떻게 확인하면 될까요?", "예: 버튼을 누르면 파일이 받아짐"],
    program: ["어느 프로그램(시스템)인가요?", ""]
  };
  var S = { programs: [], version: "", demo: false, busy: false, code: "" };

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
    }, function () { throw { code: "network", message: "담당자 PC에 연결하지 못했습니다. 담당자 프로그램이 켜져 있는지 확인한 뒤 다시 해 주세요." }; });
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
      var sent = load(SENT_KEY, []);
      if (!Array.isArray(sent)) sent = [];
      sent.unshift({ code: v.code, what: v.what, at: v.receivedAt });
      save(SENT_KEY, sent.slice(0, 30));
      ["ask-what", "ask-raw", "ask-why", "ask-when", "ask-check"].forEach(function (id) { $(id).value = ""; });
      $("ask-prog").value = "";
      note("");
      go(v.code, v, true);
    }, function (err) {
      toast(errMsg(err, "보내지 못했습니다. 잠시 뒤 다시 해 주세요."));
    }).then(function () { S.busy = false; $("ask-send").disabled = false; });
  });

  /* ---------- 내 요청 보기 ---------- */
  function steps(v) {
    var st = K.askStage(v.status);
    if (st.step < 0) return h("p", { cls: "ask-stop" }, h("span", { cls: "pill warn", text: st.label }));
    return h("ol", { cls: "ask-steps", "aria-label": "진행 단계" }, K.ASK_STEPS.map(function (label, i) {
      return h("li", { cls: i < st.step ? "done" : i === st.step ? "now" : "", "aria-current": i === st.step ? "step" : null, text: label });
    }));
  }
  function row(label, value) { return value ? [h("dt", { text: label }), h("dd", { text: value })] : null; }
  function fillForm(v) {
    var inputs = {};
    var kids = v.missing.map(function (k) {
      var el;
      if (k === "program") { el = h("select", { id: "ask-f-program" }); progOptions(el, ""); }
      else el = h("input", { type: "text", id: "ask-f-" + k, maxlength: "500", autocomplete: "off", placeholder: FIELDS[k][1] });
      inputs[k] = el;
      return h("label", { "for": "ask-f-" + k }, h("span", { text: FIELDS[k][0] }), el);
    });
    var more = h("textarea", { id: "ask-f-more", rows: "2", maxlength: "1000", placeholder: "더 알려 주실 것(선택)" });
    var send = h("button", { cls: "btn primary", type: "submit", text: v.missing.length ? "빈칸 보내기" : "덧붙이기" });
    var form = h("form", { cls: "form ask-fill", novalidate: true },
      h("p", { cls: "wide label", text: v.missing.length ? "빈칸 " + v.missing.length + "개를 채워 주시면 담당 리드에게 바로 넘어갑니다." : "더 알려 주실 것이 있으면 적어 주세요." }),
      kids, h("label", { cls: "wide", "for": "ask-f-more" }, h("span", { text: "덧붙임" }), more),
      h("div", { cls: "wide row" }, send));
    form.addEventListener("submit", function (e) {
      e.preventDefault();
      if (S.busy) return;
      var body = {}, any = false;
      Object.keys(inputs).forEach(function (k) { var x = k === "program" ? inputs[k].value : clean(inputs[k].value); if (x) { body[k] = x; any = true; } });
      var m = clean(more.value);
      if (m) { body.more = m; any = true; }
      if (!any) { toast("채운 칸이 없습니다."); return; }
      S.busy = true; send.disabled = true;
      api("PATCH", "/api/ask/" + v.code, body).then(function (nv) {
        render(nv); toast("보냈습니다. 담당자 접수함에 바로 보입니다.");
      }, function (err) {
        toast(errMsg(err, "보내지 못했습니다. 잠시 뒤 다시 해 주세요."));
        if (err && err.code === "locked") show(v.code);
      }).then(function () { S.busy = false; send.disabled = false; });
    });
    return form;
  }
  function render(v, fresh) {
    var st = K.askStage(v.status), link = linkOf(v.code);
    var box = $("ask-view");
    box.replaceChildren.apply(box, [
      fresh ? h("div", { cls: "callout good" }, h("b", { text: "보냈습니다." }), h("span", { text: " 아래 링크를 저장해 두시면 진행 상황을 언제든 보실 수 있습니다." })) : null,
      h("div", { cls: "row between" }, h("h2", { text: v.what || "(제목 없음)" }), h("span", { cls: "pill on", text: st.label })),
      steps(v),
      h("p", { cls: "ex", text: st.note }),
      h("div", { cls: "ask-link row" }, h("span", { cls: "mono", text: link }), h("button", { cls: "btn sm", type: "button", text: "링크 복사", onclick: function () { copyText(link, "링크를 복사했습니다."); } })),
      h("dl", { cls: "ask-dl" },
        row("보낸 분", v.from), row("받은 때", fmtDT(v.receivedAt)), row("프로그램", v.programName || "잘 모르겠음"),
        row("왜·누가 정했는지", v.why), row("언제까지와 이유", v.when), row("확인 방법", v.check),
        row("담당 리드에게 넘긴 때", fmtDT(v.forwardedAt)), row("끝난 때", fmtDT(v.closedAt)), row("마지막으로 보탠 때", fmtDT(v.addedAt))),
      v.raw ? h("div", { cls: "ask-raw" }, h("p", { cls: "label", text: "자세히" }), h("div", { text: v.raw })) : null,
      v.editable ? fillForm(v) : h("p", { cls: "ex", text: "담당 리드에게 넘어간 뒤에는 여기서 고칠 수 없습니다. 바뀐 것이 있으면 담당자에게 직접 알려 주세요." }),
      h("div", { cls: "row" }, h("button", { cls: "btn", type: "button", text: "새 요청 보내기", onclick: home }))].filter(Boolean));
    box.hidden = false;
    $("ask-new").hidden = true;
    document.title = (v.what || "요청") + " · 요청 보내기";
    renderMine();
  }
  function show(code, fresh) {
    note("");
    return api("GET", "/api/ask/" + code).then(function (v) { render(v, fresh); }, function (err) {
      $("ask-view").hidden = true; $("ask-new").hidden = false;
      note(errMsg(err, "이 링크의 요청을 찾지 못했습니다."));
    });
  }
  /* 화면 안에서 옮겨 다닙니다(주소도 바꿈). 웹 체험판처럼 주소를 못 바꾸는 곳에서는 화면만 바꿉니다. */
  function setPath(p) { if (window.NBD_ONLY) return; try { if (location.pathname !== p) history.pushState(null, "", p); } catch (e) { /* 주소를 못 바꿔도 화면은 바뀜 */ } }
  function go(code, v, fresh) { S.code = code; setPath("/r/" + code); if (v) render(v, fresh); else show(code); window.scrollTo(0, 0); }
  function home() {
    S.code = ""; setPath("/");
    $("ask-view").hidden = true; $("ask-new").hidden = false; note("");
    document.title = "요청 보내기";
    renderMine();
    $("ask-what").focus();
  }
  function route() {
    var m = /^\/r\/([^\/?#]+)\/?$/.exec(location.pathname);
    if (m && !window.NBD_ONLY) { S.code = m[1]; show(m[1]); } else home();
  }
  window.addEventListener("popstate", route);
  /* 다른 탭에 있다가 돌아오면 보고 있던 요청을 새로 읽습니다(빈칸을 적는 중이면 그대로 둠). */
  document.addEventListener("visibilitychange", function () {
    if (document.hidden || !S.code || S.busy || $("ask-view").hidden) return;
    var a = document.activeElement;
    if (a && $("ask-view").contains(a) && /^(INPUT|TEXTAREA|SELECT)$/.test(a.tagName)) return;
    show(S.code);
  });

  /* ---------- 이 브라우저에서 보낸 요청 ---------- */
  function renderMine() {
    var sent = load(SENT_KEY, []);
    if (!Array.isArray(sent)) sent = [];
    sent = sent.filter(function (x) { return x && K.ASK_CODE_RE.test(String(x.code)); });
    $("ask-mine").hidden = !sent.length;
    var ul = $("ask-mine-list");
    ul.replaceChildren.apply(ul, sent.map(function (x) {
      var a = h("a", { href: "/r/" + x.code, text: x.what || "(제목 없음)" });
      a.addEventListener("click", function (e) { if (e.ctrlKey || e.metaKey || e.shiftKey) return; e.preventDefault(); go(x.code); });
      return h("li", { cls: x.code === S.code ? "now" : null }, a, h("span", { cls: "ex tnum", text: fmtDT(x.at) }));
    }));
  }

  /* ---------- 시작 ---------- */
  if (!K) { note("화면 파일(core.js)을 읽지 못했습니다. 새로 고침(Ctrl+F5)해 주세요."); return; }
  var f = load(FROM_KEY, "");
  if (typeof f === "string") $("ask-from").value = f;
  api("GET", "/api/ask/form").then(function (d) {
    S.programs = Array.isArray(d && d.programs) ? d.programs : [];
    S.version = d && d.version ? String(d.version) : "";
    S.demo = !!(d && d.demo);
    progOptions($("ask-prog"), $("ask-prog").value);
    $("ask-demo").hidden = !S.demo;
    $("ask-foot").textContent = "요청 창구 · 관리 수첩 v" + (S.version || "?") + " · 이 화면에는 요청 보내기와 내 요청 보기만 있습니다";
  }, function (err) { note(errMsg(err, "담당자 PC에 연결하지 못했습니다.")); });
  route();
})();
