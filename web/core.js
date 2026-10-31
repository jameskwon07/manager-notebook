/* 관리 수첩 2 — core.js
 * DOM 없이 도는 순수 함수 모음(SPEC2 §7). 브라우저에서는 window.NBCore, node에서는 require("./core.js").
 * 시험: node --test web/test/
 * CSP: eval·new Function을 쓰지 않습니다.
 * 원칙: 사람 단위 합계는 자리 계산(slotCalc) 한 곳에만 둡니다. 개인별 진행률·응답 속도·업무량 순위는 만들지 않습니다.
 */
(function (root, factory) {
  var core = factory();
  if (typeof module === "object" && module && module.exports) module.exports = core;
  if (root) root.NBCore = core;
})(typeof window !== "undefined" ? window : null, function () {
  "use strict";
  var DAY = 864e5;
  var DEFAULT_START = "2026-10-19";
  var HOLIDAYS_DEFAULT = "2026-12-25 성탄절\n2027-01-01 신정\n2027-02-08 설날 연휴\n2027-02-09 설날 대체공휴일\n2027-03-01 삼일절\n2027-05-03 노동절 대체공휴일\n2027-05-05 어린이날\n2027-05-13 부처님오신날\n2027-07-19 제헌절 대체공휴일\n2027-08-16 광복절 대체공휴일\n2027-09-14 추석 연휴\n2027-09-15 추석\n2027-09-16 추석 연휴\n2027-10-04 개천절 대체공휴일\n2027-10-11 한글날 대체공휴일\n2027-12-27 성탄절 대체공휴일";
  var COMPANY_DEFAULT = "■ 금주 실적\n- \n■ 차주 계획\n- \n■ 이슈·요청\n- ";

  /* ---------- 이름 지우기(저장·전송 전) ---------- */
  var STOP = ["고객", "사용자", "담당자", "관리자", "여러분", "선생", "회원", "대표", "상무", "전무", "이사", "본부장", "실장", "팀장", "부장", "센터장", "사장", "과장", "차장", "대리", "주임", "사원", "상사", "부모", "교수", "기사", "고객사", "그분", "다들", "모두", "선배", "후배", "동료", "팀원", "엄마", "아빠", "아버", "어머"];
  var TITLE = "님|씨|매니저님?|책임님?|선임님?|수석님?|프로님?|대리님?|과장님?|차장님?|부장님?|주임님?|사원님?|연구원님?|리더님?|님들";
  var NAME_RE = new RegExp("(^|[\\s(\\[\"'“‘/,·])([가-힣]{2,3})\\s?(" + TITLE + ")(?=$|[^가-힣]|(?:이|가|은|는|을|를|께서|께|에게|한테|의|도|과|와|랑|이랑|이랑은|하고|만|까지|부터|로|으로|이나|나|이고|이며|이라|라|요)(?![가-힣]))", "g");
  function stripNames(s) {
    var n = 0;
    var out = String(s == null ? "" : s).replace(NAME_RE, function (m, pre, w) {
      if (STOP.indexOf(w) >= 0 || /(팀|실|부|센터|사|측|쪽)$/.test(w)) return m;
      n++; return pre + "[팀원]";
    });
    out = out.replace(/@[A-Za-z0-9가-힣._-]+/g, function () { n++; return "@[팀원]"; });
    return { text: out.trim(), n: n };
  }
  var PERSON_COLS_RE = /(담당자|보고자|작성자|Assignee|Reporter|Owner|Creator)/i;
  function personCols(s) { return PERSON_COLS_RE.test(String(s || "")); }

  /* ---------- 날짜 ---------- */
  function parseYmd(s) {
    var m = /^(\d{4})-(\d{1,2})-(\d{1,2})/.exec(String(s || ""));
    if (!m) m = /^(\d{4})-(\d{1,2})-(\d{1,2})/.exec(DEFAULT_START);
    return new Date(+m[1], +m[2] - 1, +m[3]);
  }
  function mondayOf(d) { var x = new Date(d); x.setHours(0, 0, 0, 0); x.setDate(x.getDate() - ((x.getDay() + 6) % 7)); return x; }
  function addDays(d, n) { var x = new Date(d); x.setDate(x.getDate() + n); return x; }
  function addMonths(d, n) { var x = new Date(d); x.setMonth(x.getMonth() + n); return x; }
  function pad2(n) { return String(n).padStart(2, "0"); }
  function ymd(d) { return d.getFullYear() + "-" + pad2(d.getMonth() + 1) + "-" + pad2(d.getDate()); }
  function md(d) { return (d.getMonth() + 1) + "/" + d.getDate(); }
  function hm(d) { return pad2(d.getHours()) + ":" + pad2(d.getMinutes()); }
  var WD = ["일", "월", "화", "수", "목", "금", "토"];

  /* ---------- 사이클 ---------- */
  function start0(st) { return mondayOf(parseYmd(st && st.startDate)); }
  function weekNo(now, st) { return Math.round((mondayOf(now) - start0(st)) / (7 * DAY)); }
  function stageOf(w) { if (w < 0) return "시작 전"; if (w === 0) return "0주 · 준비"; if (w <= 4) return w + "주 · 관리 바닥"; if (w <= 12) return w + "주 · 보고 없는 가시성"; return w + "주 · 사람별 조정 가능"; }
  function cyc(st) { st = st || {}; return { s: Number(st.cycleStartWeek) || 2, cw: Number(st.cycleWeeks) || 6, cd: Number(st.cooldownWeeks) || 1 }; }
  function cycleOf(w, st) {
    var k = cyc(st), L = k.cw + k.cd;
    if (w < k.s) return w >= 1 ? { idx: 0, label: "사이클 1 준비 주", cool: false } : null;
    var idx = Math.floor((w - k.s) / L) + 1, pos = (w - k.s) % L;
    return pos < k.cw ? { idx: idx, label: "사이클 " + idx + " · " + (pos + 1) + "/" + k.cw + "주", cool: false } : { idx: idx, label: "정리 주 (사이클 " + idx + " 끝)", cool: true };
  }
  function cycleStart(w, st) { var k = cyc(st), c = cycleOf(w, st); if (!c || c.idx < 1) return null; return addDays(start0(st), 7 * (k.s + (c.idx - 1) * (k.cw + k.cd))); }
  function markers(st) {
    var s = start0(st), k = cyc(st);
    return [
      { d: addDays(s, 4 * 7 + 4), label: "4주 표지 · 실행만 보기" },
      { d: addDays(s, 7 * (k.s + k.cw) - 3), label: "사이클 1 끝 · 다음 주 정리 주" },
      { d: addDays(s, 13 * 7), label: "13주 · 사람별 조정 시작 가능" },
      { d: mondayOf(addMonths(s, 6)), label: "6개월 · 결과 1차로 보기" },
      { d: mondayOf(addMonths(s, 12)), label: "12개월 · 계속할지 정하기" }
    ].sort(function (a, b) { return a.d - b.d; });
  }

  /* ---------- 근무일 시계: 토·일·쉬는 날 제외, 19~08시·쉬는 날 접수는 다음 근무일 09시부터 ---------- */
  var calCache = { src: null, cal: null };
  function cal(holidays) {
    var src = String(holidays == null ? "" : holidays);
    if (calCache.src === src && calCache.cal) return calCache.cal;
    var set = {};
    (src.match(/\d{4}-\d{2}-\d{2}/g) || []).forEach(function (d) { set[d] = 1; });
    function isWorkday(d) { var wd = d.getDay(); return wd !== 0 && wd !== 6 && !set[ymd(d)]; }
    function clockStart(t) {
      var d = new Date(t);
      if (isWorkday(d) && d.getHours() >= 8 && d.getHours() < 19) return d;
      if (isWorkday(d) && d.getHours() < 8) { d.setHours(9, 0, 0, 0); return d; }
      do { d.setDate(d.getDate() + 1); } while (!isWorkday(d));
      d.setHours(9, 0, 0, 0); return d;
    }
    function addWorkdays(d, n) { var x = new Date(d), k = 0; while (k < n) { x.setDate(x.getDate() + 1); if (isWorkday(x)) k++; } return x; }
    function workdaysBetween(a, b) { var x = new Date(a), n = 0; x.setHours(0, 0, 0, 0); var end = new Date(b); end.setHours(0, 0, 0, 0); while (x < end) { x.setDate(x.getDate() + 1); if (isWorkday(x)) n++; } return n; }
    var c = { isWorkday: isWorkday, clockStart: clockStart, addWorkdays: addWorkdays, workdaysBetween: workdaysBetween };
    calCache = { src: src, cal: c };
    return c;
  }
  function startOf(b, c) { return c.clockStart(new Date(b.receivedAt)); }
  function pausedDaysNow(b, now, c) { return (Number(b.pausedDays) || 0) + (b.pausedAt ? c.workdaysBetween(b.pausedAt, now) : 0); }
  function dueOf(b, now, c) { return c.addWorkdays(startOf(b, c), (b.kind === "c" ? 1 : 2) + pausedDaysNow(b, now, c)); }
  function onTime(b, c) { return !!b.resolvedAt && new Date(b.resolvedAt) <= dueOf(Object.assign({}, b, { pausedAt: null }), new Date(b.resolvedAt), c); }
  function clockState(b, now, c) { if (b.pausedAt) return "paused"; var dl = dueOf(b, now, c); if (dl < now) return "over"; return ymd(dl) === ymd(now) ? "soon" : "ok"; }
  function clockPct(b, now, c) { var s = startOf(b, c).getTime(), dl = dueOf(b, now, c).getTime(), span = Math.max(1, dl - s); return Math.max(0, Math.min(100, 100 * (now.getTime() - s) / span)); }
  function fmtLeft(ms) { var neg = ms < 0, m = Math.abs(ms) / 6e4, hh = Math.floor(m / 60), mm = Math.floor(m % 60); return hh + "시간 " + pad2(mm) + "분" + (neg ? " 지남" : " 남음"); }
  /* 막힘 시계 문장(접수 전달 시계 reqClock과 같은 말투). 멈춘 근무일은 기한을 미루고, 지난 날 수에서는 뺍니다. */
  function blkClock(b, now, c) {
    now = toDate(now) || new Date(); c = c || cal(HOLIDAYS_DEFAULT);
    var days = b && b.kind === "c" ? 1 : 2, p = pausedDaysNow(b, now, c);
    if (b.pausedAt) return { state: "paused", text: "시계 멈춤 · 멈춘 근무일 " + p + "일" };
    var st = startOf(b, c), dl = dueOf(b, now, c), state = clockState(b, now, c);
    var used = now > st ? Math.max(0, c.workdaysBetween(st, now) - p) : 0;
    var when = WD[dl.getDay()] + " " + md(dl) + " " + hm(dl);
    return {
      state: state, due: dl, usedDays: used,
      text: state === "over" ? "근무일 " + days + "일 넘김 · " + when + "까지였습니다" : "근무일 " + days + "일 중 " + Math.min(used, days) + "일 지남 · " + when + "까지"
    };
  }
  function fmtDue(d) { return WD[d.getDay()] + " " + md(d) + " " + hm(d) + "까지"; }

  /* ---------- 줄 나누기(칸·붙여 넣기 도우미) ---------- */
  function cleanLine(s) {
    var x = String(s == null ? "" : s).replace(/\t+/g, " · "), prev;
    do { prev = x; x = x.replace(/^\s*(?:[-*•·▪◦‣–—]+|\d{1,2}[.)]|[①-⑳]|\[ ?[xX ]?\])\s*/, ""); } while (x !== prev);
    return x
      .replace(/\s+/g, " ")
      .replace(/^[·\s]+|[·\s]+$/g, "")
      .trim();
  }
  function toLines(text) {
    if (Array.isArray(text)) return text.map(cleanLine).filter(Boolean);
    return String(text == null ? "" : text).split(/\r?\n/).map(cleanLine).filter(Boolean);
  }
  function pasteLines(text, max) {
    var lim = max || 300, names = 0, out = [];
    toLines(text).forEach(function (l) {
      if (out.length >= lim) return;
      var r = stripNames(l); names += r.n;
      if (r.text) out.push(r.text);
    });
    return { lines: out, names: names, personCols: personCols(text), cut: toLines(text).length > lim };
  }

  /* ---------- 사내 AI 지시문·응답 ---------- */
  function fillPrompt(tpl, vars) {
    var used = false;
    var body = String(tpl || "").replace(/\{\{([A-Z_]+)\}\}/g, function (m, k) { used = true; return vars[k] == null ? "" : String(vars[k]).replace(/<\//g, "< /"); });
    if (!used) body += "\n\n입력(JSON):\n" + JSON.stringify(vars, null, 1);
    return body;
  }
  function extractJson(text) {
    var s = String(text == null ? "" : text).trim();
    function tryParse(x) { try { var o = JSON.parse(x); return o && typeof o === "object" ? o : null; } catch (e) { return null; } }
    var o = tryParse(s); if (o) return o;
    var re = /```[a-zA-Z]*\s*([\s\S]*?)```/g, m;
    while ((m = re.exec(s))) { o = tryParse(m[1].trim()); if (o) return o; }
    var a = s.indexOf("{"), b = s.lastIndexOf("}");
    if (a >= 0 && b > a) { o = tryParse(s.slice(a, b + 1)); if (o) return o; }
    return null;
  }

  /* ======================================================================
   * 2.0 추가(SPEC2 §4·§5·§7)
   * ====================================================================== */
  function arr(x) { return Array.isArray(x) ? x : []; }
  function str(x) { return x == null ? "" : String(x); }
  function trimS(x) { return str(x).trim(); }
  function toDate(x) { if (x == null || x === "") return null; var d = x instanceof Date ? new Date(x.getTime()) : new Date(x); return isNaN(d.getTime()) ? null : d; }
  function posInt(x, dflt) { var n = Number(x); return isFinite(n) && n > 0 ? Math.floor(n) : dflt; }
  function inWeek(x, now) { var d = toDate(x); return !!d && ymd(mondayOf(d)) === ymd(mondayOf(now)); }
  function calOf(state, c) { return c || cal(state && state.settings && state.settings.holidays != null ? state.settings.holidays : HOLIDAYS_DEFAULT); }
  function byId(list, id) { var a = arr(list); for (var i = 0; i < a.length; i++) if (a[i] && a[i].id === id) return a[i]; return null; }

  /* ---------- 설정 기본값 ---------- */
  var REQUESTER_ROLES = ["다른 파트 개발자", "과제 PM", "다른 파트", "팀장 지시", "운영 담당", "기타"];
  var TIMELOG_KINDS = ["접수·명확화·회신", "통합 설계", "멘토링", "팀장 보고·1:1", "금요 읽기", "기타"];
  var REQ_STATUSES = ["접수", "정리 중", "리드 전달", "진행", "완료", "보류", "종료"];
  var CLOCK_STOP = ["리드 전달", "진행", "완료", "보류", "종료"];
  var DECIDER_ROLES = ["파트 리드", "프로그램 리드", "팀장", "기타"];
  var RISK_LEVELS = ["낮음", "주의", "높음"];
  var SIZES = { S: "반나절 이하", M: "근무일 1~5일", L: "사이클급" };
  var CYCLE_CHECKS = 7;
  var FORWARD_DAYS = 2;
  var ASK_WAIT_DAYS = 10;
  var INTAKE_WEEK_HOURS = 3;
  var MISSING_LABELS = { why: "왜·누가 결정", when: "언제까지와 이유", check: "확인 방법", program: "어느 프로그램" };
  var MISSING_ASK = { why: "왜 필요한지, 누가 정한 일인지", when: "언제까지 필요한지와 그 이유", check: "끝났다는 것을 어떻게 확인하면 되는지", program: "어느 프로그램(시스템)에 해당하는지" };
  var MISSING_KEYS = ["why", "when", "check", "program"];

  function defaultPeople() {
    return [{ id: "p1", label: "A", leadCap: 2 }, { id: "p2", label: "B", leadCap: 2 }, { id: "p3", label: "C", leadCap: 2 }, { id: "p4", label: "D", leadCap: 2 }];
  }
  /* content(선택): content.json. bounds.urgent_defaults가 있으면 긴급 기준 기본값으로 씁니다. */
  function defaults(content) {
    var urgent = content && content.bounds && Array.isArray(content.bounds.urgent_defaults) ? content.bounds.urgent_defaults.map(String) : [];
    return {
      startDate: DEFAULT_START, cycleStartWeek: 2, cycleWeeks: 6, cooldownWeeks: 1,
      holidays: HOLIDAYS_DEFAULT, companyTemplate: "",
      people: defaultPeople(), slotCap: 2,
      requesterRoles: REQUESTER_ROLES.slice(), urgentCriteria: urgent,
      bounds: null, seededRisks: false, onboarded: false
    };
  }
  function withDefaults(settings, content) {
    var d = defaults(content), s = settings && typeof settings === "object" && !Array.isArray(settings) ? settings : {};
    var out = Object.assign({}, d);
    Object.keys(s).forEach(function (k) { if (k !== "id" && s[k] !== undefined && s[k] !== null) out[k] = s[k]; });
    if (!/^\d{4}-\d{2}-\d{2}$/.test(str(out.startDate))) out.startDate = d.startDate;
    out.cycleStartWeek = posInt(out.cycleStartWeek, d.cycleStartWeek);
    out.cycleWeeks = posInt(out.cycleWeeks, d.cycleWeeks);
    out.cooldownWeeks = posInt(out.cooldownWeeks, d.cooldownWeeks);
    out.slotCap = posInt(out.slotCap, d.slotCap);
    if (typeof out.holidays !== "string") out.holidays = d.holidays;
    if (typeof out.companyTemplate !== "string") out.companyTemplate = "";
    ["requesterRoles", "urgentCriteria"].forEach(function (k) { out[k] = Array.isArray(out[k]) ? out[k].map(str) : d[k]; });
    if (!Array.isArray(out.people)) out.people = d.people;
    out.people = out.people.filter(function (p) { return p && typeof p === "object" && trimS(p.id); }).map(function (p) {
      return Object.assign({}, p, { id: str(p.id), label: trimS(p.label) || str(p.id), leadCap: posInt(p.leadCap, 2) });
    });
    if (!Array.isArray(out.bounds)) out.bounds = null;
    out.seededRisks = !!out.seededRisks; out.onboarded = !!out.onboarded;
    return out;
  }

  /* ---------- 이름 찾기 ---------- */
  function sortedPrograms(programs) {
    return arr(programs).map(function (p, i) { return { p: p, i: i }; }).sort(function (a, b) {
      var x = Number(a.p && a.p.order), y = Number(b.p && b.p.order);
      x = isFinite(x) ? x : 1e9; y = isFinite(y) ? y : 1e9;
      return x - y || a.i - b.i;
    }).map(function (o) { return o.p; });
  }
  function programLabel(programs, id) {
    if (!id) return "미정";
    var s = sortedPrograms(programs);
    for (var i = 0; i < s.length; i++) if (s[i] && s[i].id === id) return trimS(s[i].name) || "프로그램 " + (i + 1);
    return "미정";
  }
  function personLabel(people, id) {
    if (!id) return "미정";
    var p = byId(people, id);
    return p ? (trimS(p.label) || str(id)) : str(id);
  }
  function reqTitle(r) { var t = trimS(r && r.what); if (t) return t; t = cleanLine(str(r && r.raw).split(/\r?\n/)[0]); if (t.length > 40) t = t.slice(0, 40) + "…"; return t || "(제목 없음)"; }

  /* ---------- 접수함 ---------- */
  function missing(req) {
    req = req || {};
    return MISSING_KEYS.filter(function (k) { return !trimS(req[k]); }).map(function (k) { return MISSING_LABELS[k]; });
  }
  function isOpenReq(req) { var s = req && req.status; return s !== "완료" && s !== "종료"; }
  function reqClockStopped(req) { return !req || !!req.urgent || !!req.forwardedAt || CLOCK_STOP.indexOf(req.status) >= 0; }
  /* 전달 시계: 접수 → 리드 전달 근무일 2일. 멈춤(넘김·진행·끝남·보류)과 긴급은 null. */
  function reqClock(req, now, c) {
    if (reqClockStopped(req)) return null;
    var rec = toDate(req.receivedAt) || toDate(req.createdAt);
    if (!rec) return null;
    now = toDate(now) || new Date();
    c = c || cal(HOLIDAYS_DEFAULT);
    var st = c.clockStart(rec), due = c.addWorkdays(st, FORWARD_DAYS);
    var used = now > st ? c.workdaysBetween(st, now) : 0;
    var state = due < now ? "over" : (ymd(due) === ymd(now) ? "soon" : "ok");
    var when = WD[due.getDay()] + " " + md(due) + " " + hm(due);
    var text = state === "over"
      ? "근무일 " + FORWARD_DAYS + "일 넘김 · " + when + "까지였습니다"
      : "근무일 " + FORWARD_DAYS + "일 중 " + Math.min(used, FORWARD_DAYS) + "일 지남 · " + when + "까지";
    return { due: due, state: state, usedDays: used, text: text };
  }

  /* ---------- 자리 계산(사람 단위 합계는 여기 한 곳뿐) ---------- */
  function slotCalc(programs, people) {
    var o = {};
    arr(people).forEach(function (p) { if (p && p.id) o[p.id] = { lead: 0, sub: 0, total: 0 }; });
    function bump(id, k) { if (!id) return; if (!o[id]) o[id] = { lead: 0, sub: 0, total: 0 }; o[id][k]++; o[id].total++; }
    arr(programs).forEach(function (p) {
      if (!p || !p.active) return;
      bump(p.lead, "lead");
      if (p.subImpl) bump(p.sub, "sub");
    });
    return o;
  }
  function slotWarnings(programs, people, slotCap) {
    var cap = posInt(slotCap, 2), o = slotCalc(programs, people), w = [];
    Object.keys(o).forEach(function (id) {
      var x = o[id], pr = byId(people, id), label = personLabel(people, id), lc = posInt(pr && pr.leadCap, 2);
      if (x.total > cap) w.push({ level: "crit", personId: id, text: label + ": 활성 자리 " + x.total + "개(리드 " + x.lead + " · 서브 구현 " + x.sub + "). 상한 " + cap + ". 서브 구현 하나를 다음 사이클로 돌리거나 프로그램 하나를 대기로 돌립니다." });
      if (x.lead > lc) w.push({ level: "warn", personId: id, text: label + ": 활성 리드 " + x.lead + "개. 리드 상한 " + lc + "." });
    });
    var act = arr(programs).filter(function (p) { return p && p.active; });
    var si = act.filter(function (p) { return p.subImpl; }).length;
    if (act.length >= 8) w.push({ level: "crit", personId: null, text: "활성 프로그램 " + act.length + "개. 묶거나 멈출 프로그램을 먼저 정합니다." });
    else if (act.length >= 6 && si >= 3) w.push({ level: "warn", personId: null, text: "활성 프로그램 " + act.length + "개에 서브 구현 " + si + "개. 활성 6~7개면 서브 구현은 1~2개까지입니다." });
    return w;
  }

  /* ---------- 주·사이클 id ---------- */
  function weekId(now) { return ymd(mondayOf(toDate(now) || new Date())); }
  function cycleId(now, settings) {
    var d = toDate(now) || new Date(), w = weekNo(d, settings), c = cycleOf(w, settings);
    if (!c || c.idx < 1) return null;
    var s = cycleStart(w, settings);
    return s ? ymd(s) : null;
  }

  /* ---------- 조기 신호 6개 ---------- */
  var SIGNALS = [
    { key: "over2", label: "접수 → 리드 전달 2일 넘김", auto: true },
    { key: "blockerLate", label: "막힘 시계 넘김", auto: true },
    { key: "asked", label: "상태를 물어야 했던 순간(이번 주 물음 메모)", auto: true },
    { key: "slot3", label: "활성 자리 3개 이상인 사람", auto: true },
    { key: "reviewEmpty", label: "리뷰어가 빈 PR", auto: false },
    { key: "offMap", label: "진행 지도 밖 일이 나옴", auto: false }
  ];
  function overRequests(state, now, c) {
    return arr(state && state.requests).filter(function (r) { var k = r && isOpenReq(r) ? reqClock(r, now, c) : null; return k && k.state === "over"; });
  }
  function lateBlockers(state, now, c) {
    return arr(state && state.blockers).filter(function (b) { return b && b.state === "open" && b.receivedAt && clockState(b, now, c) === "over"; });
  }
  /* labels(선택): {key: "화면 문구"} — content.json 문구로 바꿀 때 */
  function signals(state, now, c, labels) {
    state = state || {}; now = toDate(now) || new Date(); c = calOf(state, c);
    var st = withDefaults(state.settings);
    var wk = byId(state.weeks, weekId(now)), manual = (wk && wk.signals) || {};
    var over = overRequests(state, now, c).length;
    var late = lateBlockers(state, now, c).length;
    var asked = arr(state.notes).filter(function (n) { return n && n.kind === "q" && inWeek(n.createdAt, now); }).length;
    var o = slotCalc(state.programs, st.people), heavy = Object.keys(o).filter(function (id) { return o[id].total >= 3; }).length;
    var auto = {
      over2: { on: over > 0, detail: over + "건" },
      blockerLate: { on: late > 0, detail: late + "건" },
      asked: { on: asked > 0, detail: asked + "개" },
      slot3: { on: heavy > 0, detail: heavy + "명" }
    };
    return SIGNALS.map(function (s) {
      var label = labels && typeof labels[s.key] === "string" && labels[s.key] ? labels[s.key] : s.label;
      if (s.auto) return { key: s.key, label: label, on: auto[s.key].on, auto: true, detail: auto[s.key].detail };
      var on = !!manual[s.key];
      return { key: s.key, label: label, on: on, auto: false, detail: on ? "켬" : "끔" };
    });
  }

  /* ---------- 오늘: 지금 손댈 것 ---------- */
  var LEVEL_ORDER = { crit: 0, warn: 1, info: 2 };
  function firstMondayOfMonth(d) { var x = new Date(d.getFullYear(), d.getMonth(), 1); while (x.getDay() !== 1) x.setDate(x.getDate() + 1); return x; }
  /* opts(선택): {programName(id), cycleChecks:7} */
  function todayActions(state, now, c, opts) {
    state = state || {}; now = toDate(now) || new Date(); c = calOf(state, c); opts = opts || {};
    var st = withDefaults(state.settings), out = [];
    var pname = typeof opts.programName === "function" ? opts.programName : function (id) { return programLabel(state.programs, id); };
    function add(level, tab, text, ref) { var a = { level: level, tab: tab, text: text }; if (ref) a.ref = ref; out.push(a); }
    var k = cyc(st), holdMs = (k.cw + k.cd) * 2 * 7 * DAY;

    arr(state.requests).forEach(function (r) {
      if (!r || !isOpenReq(r)) return;
      var t = reqTitle(r), m = missing(r), ref = { coll: "requests", id: r.id };
      var clk = reqClock(r, now, c);
      /* 빈칸 이름 안에 '·'가 있으므로 목록은 ' / '로 잇습니다. */
      if (clk && clk.state === "over") add("crit", "intake", "전달 기한 넘김: " + t + " — " + (m.length ? "빈칸 " + m.length + "개(" + m.join(" / ") + ")를 되묻거나 리드에게 넘깁니다." : "리드에게 넘깁니다."), ref);
      else if (r.status === "접수" && m.length) add("warn", "intake", "빈칸 " + m.length + "개: " + t + " — " + m.join(" / ") + "만 되묻습니다.", ref);
      /* 긴급 사후 등록 확인: 프로그램이 비었거나 이번 주에 받은 것만(확인하면 목록에서 빠집니다). */
      if (r.urgent && (!r.program || inWeek(r.receivedAt || r.createdAt, now))) {
        add("info", "intake", "긴급 사후 등록: " + t + " — 기술 판단은 " + (r.program ? pname(r.program) + "의 리드" : "그 프로그램의 리드") + "가 합니다. 파트 리드는 대외 소통만 맡습니다." + (r.program ? "" : " 어느 프로그램인지 채웁니다."), ref);
      }
      if (r.status === "정리 중" && r.askedAt) {
        var asked = toDate(r.askedAt), n = asked ? c.workdaysBetween(asked, now) : 0;
        if (n >= ASK_WAIT_DAYS) add("warn", "intake", "되물은 뒤 근무일 " + n + "일 답 없음: " + t + " — 보류로 돌릴지 정합니다.", ref);
      }
      if (r.status === "보류") {
        /* heldAt(보류로 바꾼 때)부터 셉니다. heldAt이 없는 옛 데이터만 되물은 때·받은 때 중 늦은 것으로 셉니다. */
        var last = toDate(r.heldAt) || [toDate(r.askedAt), toDate(r.receivedAt)].filter(Boolean).sort(function (a, b) { return b - a; })[0];
        if (last && now - last >= holdMs) add("info", "intake", "보류 " + Math.floor((now - last) / (7 * DAY)) + "주: " + t + " — 요청자에게 종료 안내를 보낼지 정합니다.", ref);
      }
    });

    slotWarnings(state.programs, st.people, st.slotCap).forEach(function (w) { add(w.level, "programs", w.text); });

    arr(state.mapitems).forEach(function (m) {
      if (!m || m.done || !trimS(m.decide)) return;
      add("info", "friday", "결정 필요: " + (trimS(m.item) || "(항목 없음)") + (m.program ? " (" + pname(m.program) + ")" : "") + " — " + trimS(m.decide), { coll: "mapitems", id: m.id });
    });

    var fm = firstMondayOfMonth(now), monthKey = ymd(now).slice(0, 7);
    if (mondayOf(now) > fm) {
      var checked = arr(state.risks).some(function (x) { return x && str(x.checkedAt).slice(0, 7) === monthKey; });
      if (!checked) add("warn", "notebook", "이번 달 리스크 표를 아직 점검하지 않았습니다 — 15분 점검합니다.");
    }

    var cid = cycleId(now, st);
    if (cid && cid === weekId(now)) {
      var need = posInt(opts.cycleChecks, CYCLE_CHECKS), cdoc = byId(state.cycles, cid), checks = (cdoc && cdoc.checks) || {}, done = 0;
      for (var i = 0; i < need; i++) if (checks[i] || checks[String(i)]) done++;
      if (done < need) {
        var co = cycleOf(weekNo(now, st), st);
        add("warn", "programs", "사이클 " + (co ? co.idx : "") + " 시작 주: 시작 체크 " + done + "/" + need + " — 자리 계산과 지침 네 칸을 확인합니다.");
      }
    }
    return out.map(function (a, i) { return { a: a, i: i }; }).sort(function (x, y) { return LEVEL_ORDER[x.a.level] - LEVEL_ORDER[y.a.level] || x.i - y.i; }).map(function (x) { return x.a; });
  }

  /* ---------- 복사 문장 ---------- */
  /* ---------- 요청 접수함(한 기능판) ---------- */
  /* 보기: open(열린 것) · all(전체) · done(끝난 것). 정렬: 열린 것은 시계 넘김 → 오늘 기한 → 여유 → 시계 없음,
     같으면 최근 접수부터. 끝난 것은 맨 뒤. */
  var INTAKE_RANK = { over: 0, soon: 1, ok: 2 };
  function intakeRank(r, now, c) {
    if (!isOpenReq(r)) return 4;
    var k = reqClock(r, now, c);
    return k && INTAKE_RANK.hasOwnProperty(k.state) ? INTAKE_RANK[k.state] : 3;
  }
  function intakeView(requests, filter, now, c) {
    var list = arr(requests).filter(function (r) {
      if (!r || typeof r !== "object") return false;
      return filter === "all" ? true : filter === "done" ? !isOpenReq(r) : isOpenReq(r);
    });
    var ranked = list.map(function (r) { return { r: r, k: intakeRank(r, now, c), t: str(r.receivedAt || r.createdAt) }; });
    ranked.sort(function (a, b) { return a.k - b.k || (a.t < b.t ? 1 : a.t > b.t ? -1 : 0); });
    return ranked.map(function (x) { return x.r; });
  }
  /* 머리 숫자: 열린 요청, 전달 기한 넘김, 빈칸 있는 접수(접수·정리 중) */
  function intakeCounts(requests, now, c) {
    var open = arr(requests).filter(function (r) { return r && isOpenReq(r); });
    var over = open.filter(function (r) { var k = reqClock(r, now, c); return k && k.state === "over"; }).length;
    var blanks = open.filter(function (r) { return (r.status === "접수" || r.status === "정리 중" || !r.status) && !r.urgent && missing(r).length > 0; }).length;
    return { open: open.length, over: over, blanks: blanks };
  }
  /* 상태를 바꿀 때 함께 적을 시각들(서버에 보낼 patch). now는 ISO 문자열. */
  function statusPatch(cur, next, nowIso) {
    cur = cur || {};
    var p = { status: next }, closed = ["완료", "종료"];
    if (next === "리드 전달" && !cur.forwardedAt) p.forwardedAt = nowIso;
    if (closed.indexOf(next) >= 0 && !cur.closedAt) p.closedAt = nowIso;
    if (closed.indexOf(next) < 0 && cur.closedAt) p.closedAt = null;
    if (next === "보류" && cur.status !== "보류") p.heldAt = nowIso;
    if (next !== "보류" && cur.heldAt) p.heldAt = null;
    return p;
  }
  function askText(req, programName) {
    req = req || {};
    var t = reqTitle(req), pn = trimS(programName), keys = MISSING_KEYS.filter(function (k) { return !trimS(req[k]); });
    var head = "안녕하세요. \"" + t + "\"" + (pn && pn !== "미정" ? " (" + pn + ")" : "") + " 요청을 접수했습니다.";
    if (!keys.length) return head + "\n빈칸이 없어 담당 리드에게 넘깁니다. 리드 추정을 받은 뒤 근무일 2일 안에 회신드리겠습니다.";
    return [head, "담당 리드에게 넘기기 전에 " + keys.length + "가지만 여쭙니다."]
      .concat(keys.map(function (k, i) { return (i + 1) + ") " + MISSING_ASK[k]; }))
      .concat(["답을 주시면 리드 추정을 받은 뒤 근무일 2일 안에 회신드리겠습니다."]).join("\n");
  }
  function handoffText(req, programName) {
    req = req || {};
    var pn = trimS(programName) || (req.program ? "" : "미정");
    var decided = [], open = [];
    decided.push("무엇: " + reqTitle(req));
    if (trimS(req.why)) decided.push("왜·누가 결정: " + trimS(req.why));
    if (trimS(req.when)) decided.push("언제까지: " + trimS(req.when));
    if (trimS(req.check)) decided.push("확인 방법: " + trimS(req.check));
    if (SIZES[req.size]) decided.push("크기: " + req.size + "(" + SIZES[req.size] + ")");
    missing(req).forEach(function (m) { open.push(m); });
    var e = req.est;
    if (e && (e.min || e.mid || e.max)) decided.push("추정: 최소 " + (str(e.min) || "-") + " · 보통 " + (str(e.mid) || "-") + " · 최대 " + (str(e.max) || "-"));
    else open.push("리드 추정(최소·보통·최대)");
    var lines = ["[접수 넘김] " + reqTitle(req) + (pn ? " — " + pn : "") + (req.urgent ? " (긴급·사후 등록)" : "")];
    lines.push("1) 결정된 것: " + decided.join(" / "));
    lines.push("2) 열린 것: " + (open.length ? open.join(" / ") : "없음"));
    lines.push("3) 요청자: " + (trimS(req.from) || "미정"));
    var raw = cleanLine(str(req.raw).replace(/\r?\n/g, " "));
    if (raw) lines.push("요청 원문: " + raw);
    lines.push("다른 프로그램에 걸치면 그쪽 리드도 함께 봐 주세요. 요청자에게 날짜는 리드 추정 뒤에만 말합니다.");
    return lines.join("\n");
  }

  /* ---------- 금요일: 주간 서면 보고 초안 ---------- */
  function absorbedThisWeek(requests, now) {
    return arr(requests).filter(function (r) { return r && r.absorbed && inWeek(r.absorbedAt || r.receivedAt, now); }).length;
  }
  function fridayCounts(state, now, c) {
    state = state || {}; now = toDate(now) || new Date(); c = calOf(state, c);
    var R = arr(state.requests);
    return {
      waiting: R.filter(function (r) { return r && (r.status === "접수" || r.status === "정리 중"); }).length,
      over: overRequests(state, now, c).length,
      newThisWeek: R.filter(function (r) { return r && inWeek(r.receivedAt, now); }).length,
      doneThisWeek: R.filter(function (r) { return r && r.status === "완료" && inWeek(r.closedAt, now); }).length,
      absorbed: absorbedThisWeek(R, now),
      active: arr(state.programs).filter(function (p) { return p && p.active; }).length
    };
  }
  /* opts(선택): {programName(id), map:[mapitems], cal} */
  function fridayDraft(state, now, opts) {
    state = state || {}; now = toDate(now) || new Date(); opts = opts || {};
    var c = calOf(state, opts.cal);
    var pname = typeof opts.programName === "function" ? opts.programName : function (id) { return programLabel(state.programs, id); };
    var map = arr(opts.map || state.mapitems).filter(function (m) { return m && !m.done; });
    function line(m, field) { return "- " + (trimS(m.item) || "(항목 없음)") + (m.program ? " (" + pname(m.program) + ")" : "") + ": " + trimS(m[field]).replace(/\s*\r?\n\s*/g, " "); }
    var risks = map.filter(function (m) { return trimS(m.risk); }).map(function (m) { return line(m, "risk"); });
    var dec = map.filter(function (m) { return trimS(m.decide); }).map(function (m) { return line(m, "decide"); });
    var n = fridayCounts(state, now, c);
    return [
      "주간 보고 (" + ymd(now) + ")",
      "",
      "1. 위험 (진행 지도 위험 칸)",
      risks.length ? risks.join("\n") : "- 없음",
      "",
      "2. 결정 필요",
      dec.length ? dec.join("\n") : "- 없음",
      "",
      "3. 접수 현황",
      "- 접수 대기 " + n.waiting + "건 · 전달 2일 초과 " + n.over + "건 · 이번 주 새 요청 " + n.newThisWeek + "건 · 이번 주 완료 " + n.doneThisWeek + "건",
      "- 활성 프로그램 " + n.active + "개",
      "",
      "4. 이번 주 흡수 건수: " + n.absorbed + "건",
      "",
      "개인별 진행률·업무량은 적지 않습니다."
    ].join("\n");
  }

  /* ---------- Confluence 붙여 넣기용 표 ---------- */
  function escHTML(s) { return str(s).replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;").replace(/"/g, "&quot;").replace(/'/g, "&#39;"); }
  function cellHTML(v) { return escHTML(v).replace(/\r?\n/g, "<br>"); }
  function tableHTML(headers, rows) {
    var h = "<tr>" + arr(headers).map(function (x) { return "<th>" + cellHTML(x) + "</th>"; }).join("") + "</tr>";
    var b = arr(rows).map(function (r) { return "<tr>" + arr(r).map(function (x) { return "<td>" + cellHTML(x) + "</td>"; }).join("") + "</tr>"; }).join("");
    return "<table><thead>" + h + "</thead><tbody>" + b + "</tbody></table>";
  }
  function cellWiki(v) {
    var s = str(v).replace(/\r\n?/g, "\n").replace(/^\n+|\n+$/g, "");
    /* 백슬래시는 위키에서 다음 글자를 이스케이프하고(\\는 줄바꿈) 열 구분자를 깨므로, 다른 이스케이프 전에 숫자 엔티티로 바꿉니다. */
    s = s.replace(/\\/g, "&#92;");
    s = s.replace(/([|\[\]{}])/g, "\\$1").replace(/\n/g, " \\\\ ");
    return s.trim() ? s : " ";
  }
  function tableWiki(headers, rows) {
    var out = ["||" + arr(headers).map(cellWiki).join("||") + "||"];
    arr(rows).forEach(function (r) { out.push("|" + arr(r).map(cellWiki).join("|") + "|"); });
    return out.join("\n");
  }

  /* ---------- 시간 일지 ---------- */
  function round2(n) { return Math.round(n * 100) / 100; }
  function weekSum(timelog, now) {
    var wid = weekId(now), by = {}, total = 0;
    arr(timelog).forEach(function (t) {
      if (!t || !/^\d{4}-\d{2}-\d{2}/.test(str(t.date))) return;
      if (ymd(mondayOf(parseYmd(t.date))) !== wid) return;
      var h = Number(t.hours); if (!isFinite(h) || h <= 0) return;
      var k = trimS(t.kind) || "기타";
      by[k] = round2((by[k] || 0) + h); total = round2(total + h);
    });
    return { week: wid, byKind: by, total: total, intake: by["접수·명확화·회신"] || 0, intakeOver: (by["접수·명확화·회신"] || 0) > INTAKE_WEEK_HOURS };
  }

  return {
    /* 1.0에서 가져온 것 */
    DAY: DAY, DEFAULT_START: DEFAULT_START, HOLIDAYS_DEFAULT: HOLIDAYS_DEFAULT, COMPANY_DEFAULT: COMPANY_DEFAULT, WD: WD,
    stripNames: stripNames, personCols: personCols,
    parseYmd: parseYmd, mondayOf: mondayOf, addDays: addDays, addMonths: addMonths, ymd: ymd, md: md, hm: hm,
    start0: start0, weekNo: weekNo, stageOf: stageOf, cycleOf: cycleOf, cycleStart: cycleStart, markers: markers, cyc: cyc,
    cal: cal, startOf: startOf, pausedDaysNow: pausedDaysNow, dueOf: dueOf, onTime: onTime, clockState: clockState, clockPct: clockPct, blkClock: blkClock, fmtLeft: fmtLeft, fmtDue: fmtDue,
    cleanLine: cleanLine, toLines: toLines, pasteLines: pasteLines,
    fillPrompt: fillPrompt, extractJson: extractJson,
    /* 2.0 */
    REQUESTER_ROLES: REQUESTER_ROLES, TIMELOG_KINDS: TIMELOG_KINDS, REQ_STATUSES: REQ_STATUSES, DECIDER_ROLES: DECIDER_ROLES, RISK_LEVELS: RISK_LEVELS,
    SIZES: SIZES, SIGNALS: SIGNALS, CYCLE_CHECKS: CYCLE_CHECKS, INTAKE_WEEK_HOURS: INTAKE_WEEK_HOURS,
    defaults: defaults, withDefaults: withDefaults,
    programLabel: programLabel, personLabel: personLabel, sortedPrograms: sortedPrograms, reqTitle: reqTitle,
    missing: missing, isOpenReq: isOpenReq, reqClock: reqClock,
    slotCalc: slotCalc, slotWarnings: slotWarnings,
    weekId: weekId, cycleId: cycleId,
    signals: signals, todayActions: todayActions,
    askText: askText, handoffText: handoffText,
    intakeView: intakeView, intakeCounts: intakeCounts, statusPatch: statusPatch,
    absorbedThisWeek: absorbedThisWeek, fridayCounts: fridayCounts, fridayDraft: fridayDraft,
    escHTML: escHTML, tableHTML: tableHTML, tableWiki: tableWiki,
    weekSum: weekSum
  };
});
