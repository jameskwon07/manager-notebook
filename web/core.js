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
  /* 담당 리드가 맡은 요청인가: 넘김 시각이 있거나 진행 중(긴급처럼 '리드 전달'을 거치지 않고 진행된 것 포함). 서버 canConfirm과 같은 기준. */
  function leadHas(req) { return !!(req && (req.forwardedAt || req.status === "진행")); }
  /* 요청자가 리드에게 넘어가기 전에 취소(withdrawnAt)하면 닫힌 것으로 봅니다. 리드가 맡은 뒤 취소는 담당자가 리드에게 알리고 닫을 때까지 열림. */
  function isOpenReq(req) { var s = req && req.status; if (s === "완료" || s === "종료") return false; return !(req && req.withdrawnAt && !leadHas(req)); }
  function reqClockStopped(req) { return !req || !!req.urgent || !!req.forwardedAt || !!req.withdrawnAt || CLOCK_STOP.indexOf(req.status) >= 0; }
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
      var nw = askNews(r);
      if (nw && (nw.level === "crit" || nw.kind === "doneok")) add(nw.level === "crit" ? "crit" : "info", "intake", "요청자 소식: " + reqTitle(r) + " — " + nw.text, { coll: "requests", id: r.id });
      if (!r || !isOpenReq(r)) return;
      var t = reqTitle(r), m = missing(r), ref = { coll: "requests", id: r.id };
      var es = etaState(r, now, c);
      if (es === "past") add("warn", "intake", "요청자에게 알린 날짜가 지남: " + t + " — 새 날짜나 한마디를 요청자 화면에 남깁니다.", ref);
      else if (es === "soon") add("info", "intake", "요청자에게 알린 날짜가 내일: " + t + " — 리드에게 그대로인지 확인합니다.", ref);
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
        /* 요청자 화면의 '…까지 답이 없으면 보류해 둡니다'(askWaitEnd)와 같은 날부터 알립니다. */
        if (asked && ymd(now) >= ymd(askWaitEnd(asked, c))) add("warn", "intake", "되물은 뒤 근무일 " + n + "일 답 없음: " + t + " — 보류로 돌릴지 정합니다.", ref);
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
    var n = askNews(r);
    if (n && n.level === "crit") return -1;
    if (!isOpenReq(r)) return reqAttention(r) ? -1 : 4;
    var k = reqClock(r, now, c);
    return k && INTAKE_RANK.hasOwnProperty(k.state) ? INTAKE_RANK[k.state] : 3;
  }
  /* keep: 지금 열어 둔 요청 id. 연 것으로 소식을 본 것으로 쳐도 그 줄이 보기에서 빠지지 않게 합니다. */
  function intakeView(requests, filter, now, c, keep) {
    var list = arr(requests).filter(function (r) {
      if (!r || typeof r !== "object") return false;
      if (keep && r.id === keep) return true;
      return filter === "all" ? true : filter === "done" ? !isOpenReq(r) : isOpenReq(r) || reqAttention(r);
    });
    var ranked = list.map(function (r) { return { r: r, k: intakeRank(r, now, c), t: str(r.receivedAt || r.createdAt) }; });
    ranked.sort(function (a, b) { return a.k - b.k || (a.t < b.t ? 1 : a.t > b.t ? -1 : 0); });
    return ranked.map(function (x) { return x.r; });
  }
  /* 머리 숫자: 열린 요청, 전달 기한 넘김, 빈칸 있는 접수(접수·정리 중), 요청자 소식(안 본 것·꼭 볼 것) */
  function intakeCounts(requests, now, c) {
    var all = arr(requests).filter(function (r) { return r && typeof r === "object"; });
    var open = all.filter(function (r) { return isOpenReq(r); });
    var over = open.filter(function (r) { var k = reqClock(r, now, c); return k && k.state === "over"; }).length;
    var blanks = open.filter(function (r) { return (r.status === "접수" || r.status === "정리 중" || !r.status) && !r.urgent && missing(r).length > 0; }).length;
    var news = all.filter(function (r) { var n = askNews(r); return n && (n.unseen || n.level === "crit"); }).length;
    return { open: open.length, over: over, blanks: blanks, news: news };
  }
  /* 상태를 바꿀 때 함께 적을 시각들(서버에 보낼 patch). now는 ISO 문자열. */
  function statusPatch(cur, next, nowIso) {
    cur = cur || {};
    var p = { status: next }, closed = ["완료", "종료"];
    /* statusAt: 지금 단계가 시작된 때(요청자 화면의 '…부터'). 같은 단계로 다시 고르면 그대로 둡니다. */
    if ((cur.status || "접수") !== next || !cur.statusAt) p.statusAt = nowIso;
    if (next === "리드 전달" && !cur.forwardedAt) p.forwardedAt = nowIso;
    if (closed.indexOf(next) >= 0 && !cur.closedAt) p.closedAt = nowIso;
    if (closed.indexOf(next) < 0 && cur.closedAt) p.closedAt = null;
    /* 끝난 요청을 다시 열면 그 전에 요청자가 남긴 '끝났어요·아직 다릅니다'는 지난 것이 됩니다. */
    if (closed.indexOf(next) < 0 && closed.indexOf(cur.status) >= 0) p.reopenedAt = nowIso;
    if (next === "보류" && cur.status !== "보류") p.heldAt = nowIso;
    if (next !== "보류" && cur.heldAt) p.heldAt = null;
    return p;
  }
  /* link: 요청 창구가 열려 있으면 요청자가 빈칸을 직접 채울 링크(없으면 메신저로 답을 받음) */
  function askText(req, programName, link) {
    req = req || {};
    link = trimS(link);
    /* 링크가 있으면 요청자 화면과 같은 빈칸만 묻습니다(어느 프로그램인지는 담당자가 고름). */
    var t = reqTitle(req), pn = trimS(programName), keys = (link ? ASK_GAP_KEYS : MISSING_KEYS).filter(function (k) { return !trimS(req[k]); });
    var head = "안녕하세요. \"" + t + "\"" + (pn && pn !== "미정" ? " (" + pn + ")" : "") + " 요청을 접수했습니다.";
    if (!keys.length) return head + "\n빈칸이 없어 근무일 " + FORWARD_DAYS + "일 안에 담당 리드에게 넘깁니다. " + DATE_PROMISE + (link ? "\n진행 상황은 이 링크에서 보실 수 있습니다: " + link : "");
    return [head, "담당 리드에게 넘기기 전에 " + keys.length + "가지만 여쭙니다."]
      .concat(keys.map(function (k, i) { return (i + 1) + ") " + MISSING_ASK[k]; }))
      .concat([link ? "이 링크에서 바로 채워 주시면 됩니다(진행 상황도 여기서 보입니다): " + link : "답을 주시면 근무일 " + FORWARD_DAYS + "일 안에 담당 리드에게 넘깁니다. " + DATE_PROMISE]).join("\n");
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
    var adds = reqAdds(req);
    if (adds.length) lines.push("요청자 덧붙임: " + adds.map(function (a) { return cleanLine(a.text.replace(/\r?\n/g, " ")); }).join(" / "));
    lines.push("다른 프로그램에 걸치면 그쪽 리드도 함께 봐 주세요. 요청자에게 날짜는 리드 추정 뒤에만 말합니다.");
    return lines.join("\n");
  }

  /* ---------- 요청 창구: 요청자가 직접 적습니다 ---------- */
  var ASK_CODE_CHARS = "23456789abcdefghjkmnpqrstuvwxyz";
  var ASK_CODE_RE = /^[23456789abcdefghjkmnpqrstuvwxyz]{10}$/;
  /* 요청자 링크 코드(서버와 같은 글자·길이). bytes: 난수 10개(crypto.getRandomValues). */
  function askCodeFrom(bytes) {
    var out = "";
    for (var i = 0; i < 10; i++) out += ASK_CODE_CHARS.charAt((Number(bytes && bytes[i]) || 0) % ASK_CODE_CHARS.length);
    return out;
  }
  function askLink(base, code) { base = trimS(base); if (!base || !code) return ""; return base.replace(/\/+$/, "") + "/r/" + code; }
  /* 메신저·메일로 요청이 왔을 때 옮겨 적지 않고 보낼 답 */
  function inviteText(url) {
    return "번거로우시겠지만 요청은 아래 링크에 한 번만 적어 주세요. 빈칸이 채워지면 근무일 " + FORWARD_DAYS + "일 안에 담당 리드에게 넘기고, 진행 상황과 날짜는 보내신 뒤 받는 링크에서 바로 보실 수 있습니다.\n" + trimS(url);
  }
  function statusLinkText(req, link) { return "\"" + reqTitle(req) + "\" 요청의 진행 상황은 이 링크에서 보실 수 있습니다: " + trimS(link); }
  /* 창구를 열 수 없을 때: 요청자가 메신저에서 채워 보낼 양식. 받은 글은 parseAskTemplate로 칸이 나뉩니다. */
  var TEMPLATE_LINES = ["무엇(한 줄): ", "왜 필요하고 누가 정했나요: ", "언제까지, 그 날짜인 이유: ", "끝났다는 확인 방법: ", "어느 프로그램: ", "보내는 분: ", "자세히: "];
  function askTemplate() { return ["[요청]"].concat(TEMPLATE_LINES).join("\n"); }
  var TEMPLATE_INTRO = "아래 양식을 채워 보내 주세요. 모르는 칸은 비워 두셔도 됩니다.";
  function templateText() { return TEMPLATE_INTRO + "\n" + askTemplate(); }
  var LABEL_KEYS = [
    ["when", /언제|기한|마감|까지/], ["check", /확인|완료\s*기준|끝났/], ["from", /보내는|보낸|요청자|요청한\s*분|작성자/],
    ["program", /프로그램|시스템/], ["why", /왜|이유|배경|결정|정했/], ["what", /무엇|제목|요청\s*사항|할\s*일|^요청$/],
    ["raw", /자세히|내용|설명|기타|참고|메모/]
  ];
  var BLANK_ANSWER = /^(?:[-–—?]+|모름|미정|(?:잘\s*)?모르겠(?:음|어요|습니다)?)$/;
  function labelKey(label) {
    label = trimS(label);
    if (!label || label.length > 24 || /https?/i.test(label)) return "";
    for (var i = 0; i < LABEL_KEYS.length; i++) if (LABEL_KEYS[i][1].test(label)) return LABEL_KEYS[i][0];
    return "";
  }
  function normName(s) { return str(s).toLowerCase().replace(/[\s·_\-()[\]]+/g, ""); }
  function matchProgram(programs, text) {
    var t = normName(text);
    if (!t) return "";
    var list = arr(programs).filter(function (p) { return p && p.id && p.active !== false && normName(p.name); });
    for (var i = 0; i < list.length; i++) if (normName(list[i].name) === t) return list[i].id;
    var part = list.filter(function (p) { var n = normName(p.name); return t.length >= 2 && (n.indexOf(t) >= 0 || t.indexOf(n) >= 0); });
    return part.length === 1 ? part[0].id : "";
  }
  /* 요청자가 채워 보낸 양식(또는 아무 글)을 칸으로 나눕니다. 양식이 아니면 첫 줄이 '무엇', 전체가 '자세히'. */
  function parseAskTemplate(text, programs) {
    var src = str(text).replace(/\r\n?/g, "\n");
    var out = { what: "", why: "", when: "", check: "", from: "", program: "", raw: "", labeled: 0 };
    var cur = "", pre = [], vals = { what: [], why: [], when: [], check: [], from: [], program: [], raw: [] };
    src.split("\n").forEach(function (line) {
      var m = /^\s*(?:[-*•·]\s*)?([^:：\n]{1,24}?)\s*[:：]\s*(.*)$/.exec(line), k = m ? labelKey(m[1]) : "";
      if (k) { cur = k; out.labeled++; if (trimS(m[2])) vals[k].push(m[2].trim()); return; }
      if (!cur && (/^\s*\[[^\]]{0,12}\]\s*$/.test(line) || trimS(line) === TEMPLATE_INTRO)) return;
      if (!trimS(line)) { if (cur === "raw") vals.raw.push(""); return; }
      if (cur) vals[cur].push(line.trim()); else pre.push(line.trim());
    });
    if (!out.labeled) {
      var all = trimS(src);
      var first = cleanLine(all.split("\n")[0] || "");
      out.what = first.length > 60 ? first.slice(0, 59) + "…" : first;
      out.raw = all;
      return out;
    }
    ["what", "why", "when", "check", "from", "program"].forEach(function (k) {
      var v = trimS(vals[k].join(" "));
      out[k] = BLANK_ANSWER.test(v) ? "" : v;
    });
    var raw = trimS(vals.raw.join("\n").replace(/\n{3,}/g, "\n\n"));
    if (BLANK_ANSWER.test(raw)) raw = "";
    if (pre.length) raw = trimS(pre.join("\n") + (raw ? "\n" + raw : ""));
    if (out.program) {
      var id = matchProgram(programs, out.program);
      if (!id) raw = trimS(raw + (raw ? "\n" : "") + "어느 프로그램(요청자가 적은 것): " + out.program);
      out.program = id;
    }
    if (out.what.length > 120) out.what = out.what.slice(0, 119) + "…";
    if (!out.what) { var f = cleanLine(raw.split("\n")[0] || ""); out.what = f.length > 60 ? f.slice(0, 59) + "…" : f; }
    out.raw = raw;
    return out;
  }
  /* ---------- 요청자에게 보이는 진행: 약속은 여기 한 곳에만 적습니다 ---------- */
  var DATE_PROMISE = "날짜는 담당 리드가 작업량을 가늠한 뒤 알려 드립니다.";
  /* 창구 첫 화면·요청 화면의 '요청은 이렇게 처리됩니다'. 지키는 약속만 적습니다. */
  var ASK_RULES = [
    "담당자가 읽고, 빈칸(왜·언제까지·확인 방법)이 있으면 이 링크로 여쭙니다.",
    "빈칸이 채워지면 근무일 " + FORWARD_DAYS + "일 안에 그 프로그램의 담당 리드에게 넘깁니다.",
    "날짜는 담당 리드가 작업량을 가늠한 뒤 이 링크에 적어 드립니다. 그 전에는 날짜를 약속하지 않습니다.",
    "보류하거나 진행하지 않기로 하면 이유를 이 링크에 적어 드립니다.",
    "넘긴 뒤에 생긴 일은 이 링크의 덧붙임에 적어 주세요. 담당자가 읽고 담당 리드에게 전합니다.",
    "오늘·내일 꼭 필요한 일은 보내신 뒤 링크와 함께 메신저로도 알려 주세요."
  ];
  /* 보류·종료 이유(요청자와 링크를 받은 누구나 봅니다). 탓하는 말 대신 무엇을 기다리는지·누가 정했는지를 적습니다.
     거절은 팀장과 정한 뒤에만(결정 경계), 그래서 '하지 않습니다'만 있는 이유는 없습니다. */
  var HOLD_REASONS = [
    "여쭤본 빈칸의 답을 기다립니다. 이 링크에서 채워 주시면 다시 이어 갑니다.",
    "이번 사이클에는 자리가 없어 다음 사이클에 다시 봅니다.",
    "팀장 판단을 기다리고 있습니다.",
    "다른 파트의 결정이나 자료를 기다리고 있습니다."
  ];
  var CLOSE_REASONS = [
    "같은 요청이 이미 있어 그쪽과 합쳤습니다.",
    "다른 파트에서 맡는 일로 확인되어 이 파트에서는 진행하지 않습니다.",
    "여쭤본 빈칸의 답을 받지 못해 닫습니다. 다시 필요하면 새로 보내 주세요.",
    "팀장과 정한 결과, 이번에는 진행하지 않습니다."
  ];
  var CANCEL_REASON = "요청하신 분이 취소했습니다.";
  /* 요청자 화면의 단계 막대. 보류·진행 안 함·취소는 막대 대신 이름표로 보입니다(step -1). */
  var ASK_STEPS = ["받음", "내용 확인", "리드에게 넘김", "작업 중", "끝남"];
  function askStage(status) {
    switch (status) {
      case "정리 중": return { step: 1, label: "내용 확인" };
      case "리드 전달": return { step: 2, label: "리드에게 넘김" };
      case "진행": return { step: 3, label: "작업 중" };
      case "완료": return { step: 4, label: "끝남" };
      case "보류": return { step: -1, label: "보류" };
      case "종료": return { step: -1, label: "진행 안 함" };
      default: return { step: 0, label: "받음" };
    }
  }
  function shortDay(x) { var d = toDate(x); return d ? md(d) + "(" + WD[d.getDay()] + ")" : ""; }
  function after(a, b) { var x = toDate(a), y = toDate(b); return !!x && (!y || x > y); }
  function latest(list) { var best = null; list.forEach(function (x) { if (after(x, best)) best = x; }); return best ? str(best) : ""; }
  /* 요청자에게 되묻는 빈칸. 어느 프로그램인지는 담당자가 고르므로 요청자의 빈칸이 아닙니다. */
  var ASK_GAP_KEYS = ["why", "when", "check"];
  function askGaps(v) {
    v = v || {};
    var keys = Array.isArray(v.missing) ? v.missing.filter(function (k) { return ASK_GAP_KEYS.indexOf(k) >= 0; }) : ASK_GAP_KEYS.filter(function (k) { return !trimS(v[k]); });
    return keys.map(function (k) { return MISSING_LABELS[k]; });
  }
  function reqAdds(r) {
    return arr(r && r.adds).filter(function (a) { return a && typeof a === "object" && trimS(a.text); })
      .map(function (a) { return { at: str(a.at), text: str(a.text), kind: str(a.kind) }; });
  }
  function etaLine(v) { return v && /^\d{4}-\d{2}-\d{2}$/.test(str(v.eta)) ? "끝날 날짜: " + shortDay(parseYmd(v.eta)) + "쯤 — 담당 리드가 가늠한 날이라 바뀔 수 있습니다." : ""; }
  /* 요청자 화면의 '지금'. v: 창구가 준 요청(publicView) 또는 수첩의 요청 문서.
     누구 차례인지(who: me 요청하신 분 · desk 담당자 · lead 담당 리드 · "" 끝)와 지킬 날짜만 말합니다.
     담당자가 요청자의 답을 기다리는 동안에는 '늦음'을 말하지 않고, 넘길 날은 마지막 빈칸이 채워진 때부터 셉니다. */
  var WHO_LABEL = { me: "요청하신 분 차례", desk: "담당자 차례", lead: "담당 리드 차례", "": "" };
  function askNow(v, now, c) {
    v = v || {}; now = toDate(now) || new Date(); c = c || cal(HOLIDAYS_DEFAULT);
    var st = v.status || "접수", gaps = askGaps(v), out = { who: "", label: "", step: 0, lines: [], due: null, late: false, since: "", last: "" };
    var pn = trimS(v.programName), toLead = (pn ? pn + " " : "") + "담당 리드";
    out.last = latest([v.receivedAt, v.statusAt, v.askedAt, v.forwardedAt, v.heldAt, v.closedAt, v.replyAt, v.etaAt, v.filledAt, v.addedAt, v.withdrawnAt, v.doneOkAt, v.doneNoAt]);
    function set(who, label, step, since, lines) { out.who = who; out.label = label; out.step = step; out.since = str(since || ""); out.lines = lines.filter(Boolean); }
    var closed = st === "완료" || st === "종료", ds = doneState(v);
    if (v.withdrawnAt && (!leadHas(v) || closed)) {
      set("", "취소함", -1, v.withdrawnAt, [shortDay(v.withdrawnAt) + "에 요청을 취소하셨습니다.", "다시 필요하면 새로 보내 주세요."]);
      return withWho(out);
    }
    if (v.withdrawnAt && !closed) {
      set("desk", "취소 알림", askStage(st).step, v.withdrawnAt, [shortDay(v.withdrawnAt) + "에 취소를 알려 주셨습니다.", "담당자가 담당 리드에게 전하고 닫습니다."]);
      return withWho(out);
    }
    switch (st) {
      case "종료":
        set("", "진행 안 함", -1, v.closedAt || v.statusAt, [shortDay(v.closedAt || v.statusAt) + "에 진행하지 않기로 했습니다.", "다시 봐 주셨으면 하는 것이 있으면 아래 덧붙임에 적어 주세요."]);
        break;
      case "완료":
        var ok = ds === "ok", no = ds === "no";
        set(ok ? "" : no ? "desk" : "me", "끝남", 4, v.closedAt || v.statusAt, [shortDay(v.closedAt || v.statusAt) + "에 끝났습니다.",
          trimS(v.check) ? "요청하실 때 적으신 확인 방법: " + trimS(v.check) : "",
          ok ? shortDay(v.doneOkAt) + "에 확인해 주셨습니다." : no ? shortDay(v.doneNoAt) + "에 다른 점을 알려 주셨습니다. 담당자가 확인합니다." : "확인해 보시고 아래에서 알려 주세요."]);
        break;
      case "보류":
        /* 빈칸 답을 기다리는 보류(이유가 HOLD_REASONS[0])이고 요청자가 채울 수 있을 때만 요청자 차례입니다. */
        var refill = canFill(v) && gaps.length > 0 && replyNow(v).indexOf(HOLD_REASONS[0]) === 0;
        set(refill ? "me" : "desk", "보류", -1, v.heldAt || v.statusAt, [(shortDay(v.heldAt || v.statusAt) ? shortDay(v.heldAt || v.statusAt) + "부터 " : "") + "보류 중입니다.",
          refill ? "빈칸을 채워 주시면 담당자가 확인해 다시 이어 갑니다." : "다시 이어 가게 되면 이 화면에 바로 보입니다."]);
        break;
      case "진행":
      case "리드 전달":
        var said = ds === "ok", go = st === "진행";
        set(said ? "desk" : "lead", go ? "작업 중" : "리드에게 넘김", go ? 3 : 2, go ? v.statusAt : v.forwardedAt || v.statusAt, [
          go ? (shortDay(v.statusAt) ? shortDay(v.statusAt) + "부터 " : "") + toLead + "가 작업하고 있습니다." : shortDay(v.forwardedAt || v.statusAt) + "에 " + toLead + "에게 넘겼습니다.",
          said ? shortDay(v.doneOkAt) + "에 끝났다고 알려 주셨습니다. 담당자가 확인하고 닫습니다." :
            etaLine(v) || (go ? "끝날 날짜는 담당 리드가 가늠한 뒤 이 화면에 적어 드립니다." : "담당 리드가 작업량을 가늠한 뒤 날짜를 이 화면에 적어 드립니다.")]);
        break;
      default:
        if (gaps.length) {
          var lines = [v.askedAt ? shortDay(v.askedAt) + "에 담당자가 빈칸 " + gaps.length + "개를 여쭸습니다(" + gaps.join(" / ") + ")." : "빈칸이 " + gaps.length + "개 있습니다(" + gaps.join(" / ") + ").",
            "채워 주시면 담당자가 확인해 근무일 " + FORWARD_DAYS + "일 안에 담당 리드에게 넘깁니다."];
          var asked = toDate(v.askedAt);
          if (asked) lines.push(shortDay(askWaitEnd(asked, c)) + "까지 답이 없으면 보류해 둡니다.");
          set("me", v.askedAt ? "답을 기다림" : "받음", v.askedAt ? 1 : 0, v.askedAt || v.receivedAt, lines);
        } else {
          var from = latest([v.receivedAt, v.filledAt]) || v.createdAt, start = toDate(from);
          out.due = start ? c.addWorkdays(c.clockStart(start), FORWARD_DAYS) : null;
          out.late = !!out.due && ymd(now) > ymd(out.due);
          set("desk", st === "정리 중" ? "내용 확인" : "받음", st === "정리 중" ? 1 : 0, v.statusAt || v.receivedAt, [
            !out.due ? "근무일 " + FORWARD_DAYS + "일 안에 담당 리드에게 넘깁니다." :
              out.late ? "넘기기로 한 날(" + shortDay(out.due) + ")이 지났습니다. 넘기면 이 화면에 바로 보입니다." : shortDay(out.due) + "까지 담당 리드에게 넘깁니다.",
            DATE_PROMISE]);
        }
    }
    return withWho(out);
  }
  /* 담당자만 보는 알린 날짜 상태: past(지남) · soon(다음 근무일) · "" */
  function etaState(r, now, c) {
    if (!r || !isOpenReq(r) || !/^\d{4}-\d{2}-\d{2}$/.test(str(r.eta))) return "";
    now = toDate(now) || new Date(); c = c || cal(HOLIDAYS_DEFAULT);
    var today = ymd(now);
    if (r.eta < today) return "past";
    return ymd(c.addWorkdays(now, 1)) >= r.eta ? "soon" : "";
  }
  /* 요청자 화면에 보일 담당자의 말: 지금 단계가 시작된 뒤(같은 때 포함)에 남긴 것만. 보류 이유가 다시 진행한 뒤에도 남아 보이지 않게. */
  function replyNow(v) {
    var t = trimS(v && v.reply);
    if (!t) return "";
    return v.statusAt && v.replyAt && after(v.statusAt, v.replyAt) ? "" : t;
  }
  function withWho(o) { o.whoLabel = WHO_LABEL[o.who] || ""; return o; }
  /* 요청자의 끝남 알림: ok(끝났어요·확인했습니다) · no(아직 다릅니다) · "". 다시 연 뒤(reopenedAt)에는 그 전 것을 셈하지 않습니다. */
  function doneState(v) {
    v = v || {};
    var okAt = after(v.doneOkAt, v.reopenedAt) ? v.doneOkAt : null, noAt = after(v.doneNoAt, v.reopenedAt) ? v.doneNoAt : null;
    return after(okAt, noAt) ? "ok" : after(noAt, okAt) ? "no" : "";
  }
  /* 요청자가 빈칸을 채울 수 있는가(서버 editable과 같은 규칙). 창구가 준 값이 있으면 그것을 씁니다. */
  function canFill(v) {
    if (v && typeof v.editable === "boolean") return v.editable;
    var st = (v && v.status) || "접수";
    return !!v && !v.urgent && !v.forwardedAt && !v.withdrawnAt && (st === "접수" || st === "정리 중" || st === "보류");
  }
  /* 되물은 뒤 근무일 10일이 차는 날(담당자에게 '보류할지 정합니다'가 뜨는 날과 같음) */
  function askWaitEnd(asked, c) { return c.addWorkdays(toDate(asked), ASK_WAIT_DAYS); }
  /* 요청자의 덧붙임·빈칸 채움 가운데 가장 늦은 것({at, fill}). adds·filledAt이 없던 옛 데이터는 addedAt으로. */
  function plainNews(r) {
    var plain = reqAdds(r).filter(function (a) { return !a.kind; }).map(function (a) { return a.at; });
    var lastAdd = latest(plain), lastFill = str(r.filledAt);
    if (!lastAdd && !lastFill && !r.withdrawnAt && !r.doneOkAt && !r.doneNoAt) lastAdd = str(r.addedAt);
    var at = latest([lastAdd, lastFill]);
    return at ? { at: at, fill: !!lastFill && !after(lastAdd, lastFill) } : null;
  }
  /* 요청자가 남긴 소식(담당자 화면의 알약 하나). 중요한 것부터: 리드가 맡은 뒤 취소 > 아직 다름 > 끝남 확인 > 취소 > 덧붙임·빈칸 채움.
     unseen: 담당자가 그 요청을 연 때(seenAt) 뒤에 생긴 것. level crit은 처리할 때까지 남습니다
     (취소: 닫을 때까지, 아직 다름: 상태를 바꾸거나 요청자에게 남길 말을 새로 적을 때까지). */
  function askNews(r) {
    if (!r || typeof r !== "object") return null;
    var st = r.status || "접수", closed = st === "완료" || st === "종료", seen = r.seenAt, ds = doneState(r);
    if (r.withdrawnAt && leadHas(r) && !closed) return { kind: "cancel", level: "crit", text: "요청자가 취소 " + shortDay(r.withdrawnAt) + " · 리드에게 알리고 닫기", unseen: after(r.withdrawnAt, seen) };
    if (ds === "no" && !r.withdrawnAt && after(r.doneNoAt, r.statusAt) && after(r.doneNoAt, r.replyAt)) return { kind: "notyet", level: "crit", text: "요청자: 아직 다릅니다 " + shortDay(r.doneNoAt), unseen: after(r.doneNoAt, seen) };
    var pn = plainNews(r);
    /* 리드가 맡은 뒤에는 넘긴 때 뒤의 것만, 끝난 뒤에는 끝난 때 뒤의 것만 새 소식입니다(그 전 것은 담당자가 이미 처리함). */
    var since = closed ? r.closedAt || r.statusAt : leadHas(r) ? r.forwardedAt || r.statusAt : null;
    var plain = pn && after(pn.at, seen) && after(pn.at, r.askedAt) && after(pn.at, since)
      ? { kind: pn.fill ? "filled" : "add", level: "good", text: (pn.fill ? "요청자가 빈칸 채움 " : "요청자가 덧붙임 ") + shortDay(pn.at), unseen: true } : null;
    if (ds === "ok" && !closed && !r.withdrawnAt) return plain && after(pn.at, r.doneOkAt) ? plain : { kind: "doneok", level: "good", text: "요청자가 끝남 확인 " + shortDay(r.doneOkAt), unseen: after(r.doneOkAt, seen) };
    if (r.withdrawnAt && !leadHas(r)) return after(r.withdrawnAt, seen) ? { kind: "cancel", level: "info", text: "요청자가 취소함 " + shortDay(r.withdrawnAt), unseen: true } : null;
    return plain;
  }
  /* 끝난 요청이라도 '열린 것' 보기에 다시 올릴 것: 꼭 볼 소식이나 안 본 덧붙임(다시 봐 달라는 말 등) */
  function reqAttention(r) { var n = askNews(r); return !!n && (n.level === "crit" || (n.unseen && n.kind !== "cancel")); }
  /* 보류·종료·소식을 요청자에게 알릴 메신저 문장(이유는 요청자 화면에도 그대로 보입니다) */
  function replyText(req, link) {
    req = req || {};
    var t = reqTitle(req), st = req.status, head = st === "보류" ? "\"" + t + "\" 요청은 잠시 보류합니다." : st === "종료" ? "\"" + t + "\" 요청은 진행하지 않기로 했습니다." : "\"" + t + "\" 요청 소식입니다.";
    link = trimS(link);
    return [head, trimS(req.reply), link ? "자세한 진행 상황은 이 링크에서 보실 수 있습니다: " + link : ""].filter(Boolean).join("\n");
  }
  function etaText(req, link) {
    req = req || {};
    var d = /^\d{4}-\d{2}-\d{2}$/.test(str(req.eta)) ? shortDay(parseYmd(req.eta)) : "";
    link = trimS(link);
    return ["\"" + reqTitle(req) + "\" 요청은 담당 리드가 가늠해 보니 " + (d || "(날짜)") + "쯤 끝날 것 같습니다. 일을 해 보며 바뀔 수 있습니다.",
      link ? "진행 상황은 이 링크에서 보실 수 있습니다: " + link : ""].filter(Boolean).join("\n");
  }
  /* 리드에게 넘긴 뒤 요청자가 덧붙인 말을 리드에게 전할 문장(since 뒤의 것만) */
  function addsText(req, since) {
    var list = reqAdds(req).filter(function (a) { return after(a.at, since); });
    if (!list.length) return "";
    var tag = { cancel: " (취소)", notyet: " (아직 다름)" };
    return ["[요청자 덧붙임] " + reqTitle(req)].concat(list.map(function (a) { return "- " + shortDay(a.at) + (tag[a.kind] || "") + ": " + cleanLine(a.text.replace(/\r?\n/g, " ")); })).join("\n");
  }
  /* 요청자가 자기 윗사람에게 보고할 한 줄(요청자 화면에서 복사) */
  function askReportText(v, link, now, c) {
    v = v || {};
    var n = askNow(v, now, c), lead = n.who === "lead";
    var date = !lead ? "" : /^\d{4}-\d{2}-\d{2}$/.test(str(v.eta)) ? shortDay(parseYmd(v.eta)) + "쯤(담당 리드 추정)" : "담당 리드가 가늠한 뒤 정해짐";
    return "[요청 현황] " + reqTitle(v) + " — 지금: " + n.label + (shortDay(n.since) ? "(" + shortDay(n.since) + ")" : "") + (date ? " · 날짜: " + date : "") + (trimS(link) ? " · " + trimS(link) : "");
  }

  /* ---------- 금요일: 주간 서면 보고 초안 ---------- */
  function absorbedThisWeek(requests, now) {
    return arr(requests).filter(function (r) { return r && r.absorbed && inWeek(r.absorbedAt || r.receivedAt, now); }).length;
  }
  function fridayCounts(state, now, c) {
    state = state || {}; now = toDate(now) || new Date(); c = calOf(state, c);
    var R = arr(state.requests);
    return {
      waiting: R.filter(function (r) { return r && (r.status === "접수" || r.status === "정리 중") && !r.withdrawnAt; }).length,
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
    ASK_CODE_RE: ASK_CODE_RE, ASK_STEPS: ASK_STEPS, askCodeFrom: askCodeFrom, askLink: askLink, inviteText: inviteText, statusLinkText: statusLinkText,
    DATE_PROMISE: DATE_PROMISE, ASK_RULES: ASK_RULES, HOLD_REASONS: HOLD_REASONS, CLOSE_REASONS: CLOSE_REASONS, CANCEL_REASON: CANCEL_REASON, ASK_GAP_KEYS: ASK_GAP_KEYS,
    askGaps: askGaps, reqAdds: reqAdds, askNow: askNow, askNews: askNews, etaState: etaState, replyNow: replyNow,
    leadHas: leadHas, doneState: doneState, canFill: canFill, reqAttention: reqAttention, shortDay: shortDay,
    replyText: replyText, etaText: etaText, addsText: addsText, askReportText: askReportText,
    askTemplate: askTemplate, templateText: templateText, parseAskTemplate: parseAskTemplate, matchProgram: matchProgram, askStage: askStage,
    intakeView: intakeView, intakeCounts: intakeCounts, statusPatch: statusPatch,
    absorbedThisWeek: absorbedThisWeek, fridayCounts: fridayCounts, fridayDraft: fridayDraft,
    escHTML: escHTML, tableHTML: tableHTML, tableWiki: tableWiki,
    weekSum: weekSum
  };
});
