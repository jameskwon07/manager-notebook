// 순수 함수 시험: node --test web/test/
// 근무일 시계는 로컬 시간대로 계산하므로 Asia/Seoul로 고정합니다(첫 Date 사용 전에 지정).
process.env.TZ = "Asia/Seoul";
const test = require("node:test");
const assert = require("node:assert/strict");
const K = require("../core.js");

const D = (y, m, d, hh = 0, mm = 0) => new Date(y, m - 1, d, hh, mm);
const fmt = (d) => K.ymd(d) + " " + K.hm(d);
const iso = (...a) => D(...a).toISOString();
const CAL = K.cal(K.HOLIDAYS_DEFAULT);
const ST = { startDate: "2026-10-19", cycleStartWeek: 2, cycleWeeks: 6, cooldownWeeks: 1 };

/* ======================================================================
 * 1.0에서 옮긴 함수
 * ====================================================================== */

test("stripNames: 호칭이 붙은 이름과 @멘션을 지운다", () => {
  let r = K.stripNames("홍길동님이 배포 권한을 요청했어요");
  assert.equal(r.text, "[팀원]이 배포 권한을 요청했어요");
  assert.equal(r.n, 1);
  r = K.stripNames("아무개 대리가 @someone.x 에게 넘김");
  assert.equal(r.text, "[팀원]가 @[팀원] 에게 넘김");
  assert.equal(r.n, 2);
});

test("stripNames: 직함·일반 명사는 남긴다", () => {
  const r = K.stripNames("팀장님이 고객님께 회신, 디자인팀 님 아님");
  assert.equal(r.n, 0);
  assert.equal(r.text, "팀장님이 고객님께 회신, 디자인팀 님 아님");
});

test("근무일 시계: 금 17시 막힘 → 화 17시", () => {
  const b = { kind: "b", receivedAt: iso(2026, 10, 23, 17) };
  assert.equal(fmt(K.dueOf(b, D(2026, 10, 23, 18), CAL)), "2026-10-27 17:00");
});

test("근무일 시계: 목 11시 헷갈림 → 금 11시", () => {
  const b = { kind: "c", receivedAt: iso(2026, 10, 22, 11) };
  assert.equal(fmt(K.dueOf(b, D(2026, 10, 22, 12), CAL)), "2026-10-23 11:00");
});

test("근무일 시계: 19시 이후는 다음 근무일 09시부터, 08시 이전은 그날 09시부터", () => {
  const c = K.cal("");
  assert.equal(fmt(c.clockStart(D(2026, 10, 23, 20))), "2026-10-26 09:00");
  assert.equal(fmt(c.clockStart(D(2026, 10, 21, 7, 30))), "2026-10-21 09:00");
  assert.equal(fmt(c.clockStart(D(2026, 10, 24, 11))), "2026-10-26 09:00"); // 토요일
});

test("근무일 시계: 쉬는 날(성탄절 2026-12-25 금)을 건너뛴다", () => {
  const b = { kind: "c", receivedAt: iso(2026, 12, 24, 11) };
  assert.equal(fmt(K.dueOf(b, D(2026, 12, 24, 12), CAL)), "2026-12-28 11:00");
  assert.equal(CAL.isWorkday(D(2026, 12, 25)), false);
});

test("근무일 시계: 멈춘 동안 지난 근무일만큼 기한이 밀리고, 상태가 바뀐다", () => {
  const c = K.cal("");
  const b = { kind: "b", receivedAt: iso(2026, 10, 26, 10), pausedAt: iso(2026, 10, 27, 10), pausedDays: 0 };
  assert.equal(K.clockState(b, D(2026, 10, 29, 10), c), "paused");
  assert.equal(K.pausedDaysNow(b, D(2026, 10, 29, 10), c), 2);
  assert.equal(fmt(K.dueOf(b, D(2026, 10, 29, 10), c)), "2026-10-30 10:00");
  const open = { kind: "b", receivedAt: iso(2026, 10, 26, 10) };
  assert.equal(K.clockState(open, D(2026, 10, 27, 9), c), "ok");
  assert.equal(K.clockState(open, D(2026, 10, 28, 9), c), "soon");
  assert.equal(K.clockState(open, D(2026, 10, 28, 11), c), "over");
  assert.equal(K.onTime(Object.assign({}, open, { resolvedAt: iso(2026, 10, 28, 9) }), c), true);
  assert.equal(K.onTime(Object.assign({}, open, { resolvedAt: iso(2026, 10, 28, 11) }), c), false);
  assert.ok(K.clockPct(open, D(2026, 10, 27, 10), c) > 0);
  assert.equal(K.fmtDue(D(2026, 10, 27, 17)), "화 10/27 17:00까지");
  assert.equal(K.fmtLeft(-90 * 6e4), "1시간 30분 지남");
});

test("사이클: 기본 설정이 결정 메모 날짜와 맞는다", () => {
  assert.equal(K.weekNo(D(2026, 11, 2, 10), ST), 2);
  assert.equal(K.cycleOf(2, ST).label, "사이클 1 · 1/6주");
  assert.equal(K.cycleOf(8, ST).cool, true); // 12-14 주 정리 주
  assert.equal(K.cycleOf(9, ST).label, "사이클 2 · 1/6주"); // 12-21
  assert.equal(K.ymd(K.cycleStart(9, ST)), "2026-12-21");
  assert.equal(K.cycleOf(0, ST), null);
  assert.equal(K.cycleOf(1, ST).idx, 0);
});

test("날짜 도우미: parseYmd·addMonths·md·WD", () => {
  assert.equal(K.ymd(K.parseYmd("2026-10-19")), "2026-10-19");
  assert.equal(K.ymd(K.parseYmd("잘못된 값")), K.DEFAULT_START);
  assert.equal(K.ymd(K.addMonths(D(2026, 10, 19), 3)), "2027-01-19");
  assert.equal(K.md(D(2026, 10, 27)), "10/27");
  assert.equal(K.WD[D(2026, 10, 25).getDay()], "일");
});

test("줄 나누기: 글머리표·탭·이름을 정리한다", () => {
  const r = K.pasteLines("- 홍길동님이 결제 이관 끝냄\n\nPROJ-1\t로그인 개선\t\t진행 중\t\n• [ ] 남은 것\n담당자\t요약");
  assert.deepEqual(r.lines, ["[팀원]이 결제 이관 끝냄", "PROJ-1 · 로그인 개선 · 진행 중", "남은 것", "담당자 · 요약"]);
  assert.equal(r.names, 1);
  assert.equal(r.personCols, true);
  assert.deepEqual(K.toLines("1) 가\n② 나\n\n"), ["가", "나"]);
  assert.equal(K.cleanLine("  - [x]  끝남 "), "끝남");
});

test("JSON 꺼내기: 전체 → 코드 블록 → 첫 { 부터 마지막 } 까지", () => {
  assert.deepEqual(K.extractJson('{"a":1}'), { a: 1 });
  assert.deepEqual(K.extractJson('설명\n```json\n{"a":2}\n```\n끝'), { a: 2 });
  assert.deepEqual(K.extractJson('답: {"a":{"b":3}} 입니다'), { a: { b: 3 } });
  assert.equal(K.extractJson("JSON 없음"), null);
  assert.equal(K.extractJson(""), null);
});

test("지시문 채우기: 자리표시자를 채우고 닫는 태그를 무력화한다", () => {
  const s = K.fillPrompt("<a>{{X}}</a>", { X: "x</a>y" });
  assert.equal(s, "<a>x< /a>y</a>");
  assert.match(K.fillPrompt("자리표시자 없음", { X: 1 }), /입력\(JSON\)/);
  assert.equal(K.fillPrompt("{{DRAFT}}", { DRAFT: "초안" }), "초안");
});

/* ======================================================================
 * 2.0 함수
 * ====================================================================== */

test("defaults: §4 기본값을 매번 새 객체로 준다", () => {
  const a = K.defaults(), b = K.defaults();
  assert.notEqual(a, b);
  assert.notEqual(a.people, b.people);
  assert.equal(a.startDate, "2026-10-19");
  assert.deepEqual([a.cycleStartWeek, a.cycleWeeks, a.cooldownWeeks, a.slotCap], [2, 6, 1, 2]);
  assert.equal(a.holidays, K.HOLIDAYS_DEFAULT);
  assert.deepEqual(a.people, [{ id: "p1", label: "A", leadCap: 2 }, { id: "p2", label: "B", leadCap: 2 }, { id: "p3", label: "C", leadCap: 2 }, { id: "p4", label: "D", leadCap: 2 }]);
  assert.deepEqual(a.requesterRoles, ["다른 파트 개발자", "과제 PM", "다른 파트", "팀장 지시", "운영 담당", "기타"]);
  assert.equal(a.bounds, null);
  assert.equal(a.seededRisks, false);
  a.people.push({ id: "x" });
  assert.equal(K.defaults().people.length, 4);
  assert.deepEqual(K.defaults({ bounds: { urgent_defaults: ["가", "나"] } }).urgentCriteria, ["가", "나"]);
});

test("withDefaults: 저장값을 기본값 위에 얹고, 배열은 저장값을 쓴다", () => {
  const s = K.withDefaults({ startDate: "2026-11-02", cycleWeeks: "4", people: [{ id: "p1", label: "가" }, { id: "p9", label: "", leadCap: 1 }], requesterRoles: [], holidays: "", extra: 1, id: "settings" });
  assert.equal(s.startDate, "2026-11-02");
  assert.equal(s.cycleWeeks, 4);
  assert.equal(s.cycleStartWeek, 2);
  assert.deepEqual(s.people, [{ id: "p1", label: "가", leadCap: 2 }, { id: "p9", label: "p9", leadCap: 1 }]);
  assert.deepEqual(s.requesterRoles, []);
  assert.equal(s.holidays, ""); // 쉬는 날을 비운 것도 저장값
  assert.equal(s.extra, 1);
  assert.equal(s.id, undefined);
  assert.equal(s.slotCap, 2);
});

test("withDefaults: null·빠진 키·잘못된 값에도 죽지 않는다", () => {
  assert.deepEqual(K.withDefaults(null), K.defaults());
  assert.deepEqual(K.withDefaults(undefined), K.defaults());
  const s = K.withDefaults({ startDate: "어제", cycleWeeks: -1, cooldownWeeks: 0, slotCap: "x", people: "A,B", bounds: "x", holidays: null });
  assert.equal(s.startDate, "2026-10-19");
  assert.equal(s.cycleWeeks, 6);
  assert.equal(s.cooldownWeeks, 1); // 정리 주는 1~4주. 0은 기본 1주(사이클 계산·일정 파일과 같게)
  assert.equal(s.slotCap, 2);
  assert.equal(s.people.length, 4);
  assert.equal(s.bounds, null);
  assert.equal(s.holidays, K.HOLIDAYS_DEFAULT);
  // 1.0 설정(2.0 키 없음)
  const v1 = K.withDefaults({ startDate: "2026-10-19", cycleStartWeek: 2, cycleWeeks: 6, cooldownWeeks: 1, holidays: "2026-12-25 성탄절", companyTemplate: "■ 금주 실적" });
  assert.equal(v1.companyTemplate, "■ 금주 실적");
  assert.equal(v1.people.length, 4);
  assert.equal(v1.slotCap, 2);
});

test("missing: 빈칸 네 개를 정해진 이름·순서로 돌려준다", () => {
  assert.deepEqual(K.missing({}), ["왜·누가 결정", "언제까지와 이유", "확인 방법", "어느 프로그램"]);
  assert.deepEqual(K.missing({ why: "팀장 지시", when: "  ", check: "시험 보고서", program: "" }), ["언제까지와 이유", "어느 프로그램"]);
  assert.deepEqual(K.missing({ why: "a", when: "b", check: "c", program: "p1" }), []);
  assert.deepEqual(K.missing(null).length, 4);
});

test("isOpenReq: 완료·종료만 닫힌 것", () => {
  for (const s of ["접수", "정리 중", "리드 전달", "진행", "보류"]) assert.equal(K.isOpenReq({ status: s }), true, s);
  assert.equal(K.isOpenReq({ status: "완료" }), false);
  assert.equal(K.isOpenReq({ status: "종료" }), false);
});

test("reqClock: 금 17시 접수 → 화 17시 기한, usedDays 0/1/2/3", () => {
  const r = { status: "접수", receivedAt: iso(2026, 10, 23, 17) };
  let k = K.reqClock(r, D(2026, 10, 23, 18), CAL);
  assert.equal(fmt(k.due), "2026-10-27 17:00");
  assert.equal(k.usedDays, 0);
  assert.equal(k.state, "ok");
  assert.equal(k.text, "근무일 2일 중 0일 지남 · 화 10/27 17:00까지");
  k = K.reqClock(r, D(2026, 10, 25, 12), CAL); // 일요일
  assert.equal(k.usedDays, 0);
  k = K.reqClock(r, D(2026, 10, 26, 10), CAL);
  assert.equal(k.usedDays, 1);
  assert.equal(k.state, "ok");
  assert.equal(k.text, "근무일 2일 중 1일 지남 · 화 10/27 17:00까지");
  k = K.reqClock(r, D(2026, 10, 27, 10), CAL);
  assert.equal(k.usedDays, 2);
  assert.equal(k.state, "soon");
  k = K.reqClock(r, D(2026, 10, 28, 10), CAL);
  assert.equal(k.usedDays, 3);
  assert.equal(k.state, "over");
  assert.match(k.text, /^근무일 2일 넘김 · 화 10\/27 17:00/);
});

test("reqClock: 19시 이후 접수는 다음 근무일 09시부터", () => {
  const r = { status: "정리 중", receivedAt: iso(2026, 10, 22, 20) }; // 목 20시
  const k = K.reqClock(r, D(2026, 10, 22, 21), CAL);
  assert.equal(fmt(k.due), "2026-10-27 09:00"); // 금 09시 + 근무일 2일
  assert.equal(k.usedDays, 0);
  assert.equal(K.reqClock({ status: "접수", receivedAt: iso(2026, 10, 24, 11) }, D(2026, 10, 24, 12), CAL).due.getDate(), 28); // 토 접수 → 월 09시부터
});

test("reqClock: 쉬는 날이 끼면 그만큼 밀린다", () => {
  const r = { status: "접수", receivedAt: iso(2026, 12, 24, 11) }; // 목, 다음 날 성탄절
  let k = K.reqClock(r, D(2026, 12, 24, 12), CAL);
  assert.equal(fmt(k.due), "2026-12-29 11:00");
  k = K.reqClock(r, D(2026, 12, 28, 10), CAL);
  assert.equal(k.usedDays, 1);
  assert.equal(k.text, "근무일 2일 중 1일 지남 · 화 12/29 11:00까지");
  // 쉬는 날이 없으면 월요일 기한
  assert.equal(fmt(K.reqClock(r, D(2026, 12, 24, 12), K.cal("")).due), "2026-12-28 11:00");
});

test("reqClock: 멈춤 상태·넘김·긴급은 null", () => {
  const base = { receivedAt: iso(2026, 10, 23, 17) };
  for (const s of ["리드 전달", "진행", "완료", "보류", "종료"]) assert.equal(K.reqClock(Object.assign({ status: s }, base), D(2026, 10, 28), CAL), null, s);
  assert.equal(K.reqClock(Object.assign({ status: "정리 중", forwardedAt: iso(2026, 10, 26, 9) }, base), D(2026, 10, 28), CAL), null);
  assert.equal(K.reqClock(Object.assign({ status: "접수", urgent: true }, base), D(2026, 10, 28), CAL), null);
  assert.equal(K.reqClock({ status: "접수" }, D(2026, 10, 28), CAL), null); // 접수 시각 없음
  assert.equal(K.reqClock(null, D(2026, 10, 28), CAL), null);
  assert.notEqual(K.reqClock(Object.assign({ status: "정리 중" }, base), D(2026, 10, 28), CAL), null);
});

const PEOPLE = K.defaults().people;
const prog = (id, lead, sub, active, subImpl, extra) => Object.assign({ id, order: 0, name: "", lead, sub, active, subImpl }, extra || {});

test("slotCalc: 활성만, 서브는 서브 구현일 때만 센다", () => {
  const P = [prog("a", "p1", "p2", true, true), prog("b", "p1", "p3", true, false), prog("c", "p2", "p1", false, true), prog("d", "", "", true, true)];
  const o = K.slotCalc(P, PEOPLE);
  assert.deepEqual(o.p1, { lead: 2, sub: 0, total: 2 });
  assert.deepEqual(o.p2, { lead: 0, sub: 1, total: 1 });
  assert.deepEqual(o.p3, { lead: 0, sub: 0, total: 0 });
  assert.deepEqual(o.p4, { lead: 0, sub: 0, total: 0 });
  assert.deepEqual(Object.keys(o).sort(), ["p1", "p2", "p3", "p4"]);
  assert.deepEqual(K.slotCalc(null, null), {});
});

test("slotWarnings: 자리 상한 초과(crit)와 리드 상한 초과(warn)", () => {
  const P = [prog("a", "p1", "p2", true, false), prog("b", "p1", "p3", true, false), prog("c", "p2", "p1", true, true)];
  let w = K.slotWarnings(P, PEOPLE, 2);
  assert.equal(w.length, 1);
  assert.equal(w[0].level, "crit");
  assert.equal(w[0].personId, "p1");
  assert.ok(w[0].text.startsWith("A: 활성 자리 3개(리드 2 · 서브 구현 1). 상한 2."));
  // 상한을 3으로 올리면 경고 없음
  assert.deepEqual(K.slotWarnings(P, PEOPLE, 3), []);
  // 리드 상한 1
  const people = PEOPLE.map((p) => (p.id === "p1" ? Object.assign({}, p, { leadCap: 1 }) : p));
  w = K.slotWarnings(P, people, 3);
  assert.deepEqual(w, [{ level: "warn", personId: "p1", text: "A: 활성 리드 2개. 리드 상한 1." }]);
  // 사람 표시는 'A: …' 형식(조사 없음)
  assert.ok(K.slotWarnings(P, people, 2).every((x) => /^[A-D]: /.test(x.text)));
});

test("slotWarnings: 활성 8개면 crit, 활성 6개 + 서브 구현 3개면 warn", () => {
  const ppl = ["p1", "p2", "p3", "p4"];
  const many = (n, si) => Array.from({ length: n }, (_, i) => prog("x" + i, ppl[i % 4], "", true, i < si));
  let w = K.slotWarnings(many(8, 0), PEOPLE, 2);
  assert.deepEqual(w, [{ level: "crit", personId: null, text: "활성 프로그램 8개. 묶거나 멈출 프로그램을 먼저 정합니다." }]);
  w = K.slotWarnings(many(8, 3), PEOPLE, 2).filter((x) => x.personId === null);
  assert.equal(w.length, 1); // 8개면 crit 하나만
  assert.equal(w[0].level, "crit");
  w = K.slotWarnings(many(6, 3), PEOPLE, 2);
  assert.deepEqual(w, [{ level: "warn", personId: null, text: "활성 프로그램 6개에 서브 구현 3개. 활성 6~7개면 서브 구현은 1~2개까지입니다." }]);
  assert.deepEqual(K.slotWarnings(many(6, 2), PEOPLE, 2), []);
  assert.deepEqual(K.slotWarnings(many(5, 3), PEOPLE, 2), []);
  assert.equal(K.slotWarnings(many(7, 4), PEOPLE, 2).filter((x) => x.personId === null)[0].level, "warn");
  // 비활성은 세지 않음
  assert.deepEqual(K.slotWarnings(many(8, 0).map((p, i) => (i < 3 ? Object.assign(p, { active: false }) : p)), PEOPLE, 2), []);
});

test("weekId: 그 주 월요일, 일요일은 지난 월요일", () => {
  assert.equal(K.weekId(D(2026, 10, 25, 23, 59)), "2026-10-19"); // 일
  assert.equal(K.weekId(D(2026, 10, 26, 0, 0)), "2026-10-26"); // 월
  assert.equal(K.weekId(D(2026, 10, 23, 17)), "2026-10-19"); // 금
  assert.equal(K.weekId(D(2027, 1, 3)), "2026-12-28"); // 해 넘김 일요일
});

test("cycleId: 시작 전·준비 주는 null, 정리 주는 그 사이클 시작", () => {
  assert.equal(K.cycleId(D(2026, 10, 1), ST), null); // 시작일 전
  assert.equal(K.cycleId(D(2026, 10, 21), ST), null); // 0주
  assert.equal(K.cycleId(D(2026, 10, 28), ST), null); // 1주(준비)
  assert.equal(K.cycleId(D(2026, 11, 2, 9), ST), "2026-11-02");
  assert.equal(K.cycleId(D(2026, 11, 8), ST), "2026-11-02"); // 일요일
  assert.equal(K.cycleId(D(2026, 12, 16), ST), "2026-11-02"); // 정리 주
  assert.equal(K.cycleId(D(2026, 12, 21), ST), "2026-12-21");
  assert.equal(K.cycleId(D(2026, 11, 2), null), "2026-11-02"); // 설정 없음 → 기본
});

// 시험용 state: 2026-10-28(수) 10시 기준
const NOW = D(2026, 10, 28, 10);
function baseState(extra) {
  return Object.assign({
    settings: K.withDefaults({}), notes: [], blockers: [], requests: [], programs: [], mapitems: [], timelog: [], weeks: [], cycles: [], risks: [], decisions: [], fridays: []
  }, extra || {});
}

test("signals: 키 6개, 자동 4개는 계산하고 수동 2개는 weeks 문서에서 읽는다", () => {
  let s = K.signals(baseState(), NOW, CAL);
  assert.deepEqual(s.map((x) => x.key), ["over2", "blockerLate", "asked", "slot3", "reviewEmpty", "offMap"]);
  assert.deepEqual(s.map((x) => x.auto), [true, true, true, true, false, false]);
  assert.ok(s.every((x) => x.on === false && typeof x.label === "string" && x.label && typeof x.detail === "string"));

  const st = baseState({
    requests: [
      { id: "r1", status: "접수", receivedAt: iso(2026, 10, 23, 17) }, // 화 17시 기한 → 넘김
      { id: "r2", status: "보류", receivedAt: iso(2026, 10, 1, 10) }, // 멈춤
      { id: "r3", status: "접수", urgent: true, receivedAt: iso(2026, 10, 1, 10) } // 긴급
    ],
    blockers: [
      { id: "b1", kind: "c", state: "open", receivedAt: iso(2026, 10, 26, 10) }, // 화 10시 기한 → 넘김
      { id: "b2", kind: "b", state: "done", receivedAt: iso(2026, 10, 1, 10) },
      { id: "b3", kind: "b", state: "open", receivedAt: iso(2026, 10, 1, 10), pausedAt: iso(2026, 10, 2, 10) }
    ],
    notes: [
      { id: "n1", kind: "q", text: "x", createdAt: iso(2026, 10, 26, 9) },
      { id: "n2", kind: "q", text: "x", createdAt: iso(2026, 10, 25, 9) }, // 지난 주(일)
      { id: "n3", kind: "l", text: "x", createdAt: iso(2026, 10, 27, 9) }
    ],
    programs: [prog("a", "p1", "p2", true, false), prog("b", "p1", "", true, false), prog("c", "p2", "p1", true, true)],
    weeks: [{ id: "2026-10-26", signals: { reviewEmpty: true }, note: "" }, { id: "2026-10-19", signals: { offMap: true } }]
  });
  s = K.signals(st, NOW, CAL);
  const by = Object.fromEntries(s.map((x) => [x.key, x]));
  assert.equal(by.over2.on, true); assert.equal(by.over2.detail, "1건");
  assert.equal(by.blockerLate.on, true); assert.equal(by.blockerLate.detail, "1건");
  assert.equal(by.asked.on, true); assert.equal(by.asked.detail, "1개");
  assert.equal(by.slot3.on, true); assert.equal(by.slot3.detail, "1명");
  assert.equal(by.reviewEmpty.on, true);
  assert.equal(by.offMap.on, false); // 지난 주 문서는 보지 않음
  // labels로 문구 바꾸기
  assert.equal(K.signals(st, NOW, CAL, { asked: "물음 메모" })[2].label, "물음 메모");
  // settings가 비어도 죽지 않음
  assert.equal(K.signals({ requests: [{ status: "접수", receivedAt: iso(2026, 10, 23, 17) }] }, NOW).length, 6);
});

test("todayActions: 전달 넘김(crit)·빈칸(warn)·긴급 확인(info)·자리·결정 필요, 위험 순서로", () => {
  const st = baseState({
    programs: [prog("pa", "p1", "p2", true, true, { order: 1, name: "" }), prog("pb", "p1", "", true, false, { order: 2, name: "보고 도구" }), prog("pc", "p2", "p1", true, true, { order: 3 })],
    requests: [
      { id: "r1", what: "로그 형식 바꾸기", status: "접수", receivedAt: iso(2026, 10, 23, 17), why: "", when: "", check: "", program: "pa" },
      { id: "r2", what: "버튼 추가", status: "접수", receivedAt: iso(2026, 10, 28, 9), why: "a", when: "b", check: "", program: "pb" },
      { id: "r3", what: "장애 대응", status: "진행", urgent: true, receivedAt: iso(2026, 10, 28, 8), program: "pb" },
      { id: "r4", what: "끝난 긴급", status: "완료", urgent: true, receivedAt: iso(2026, 10, 1, 8) },
      { id: "r5", what: "빈칸 있는 정리 중", status: "정리 중", receivedAt: iso(2026, 10, 28, 9), askedAt: iso(2026, 10, 28, 9) }
    ],
    mapitems: [
      { id: "m1", item: "화면 개편", program: "pb", decide: "범위 줄일지", done: false },
      { id: "m2", item: "끝난 일", program: "pb", decide: "x", done: true },
      { id: "m3", item: "결정 없음", decide: "", done: false }
    ],
    risks: [{ id: "k1", checkedAt: "2026-10-05" }]
  });
  const a = K.todayActions(st, NOW, CAL);
  assert.ok(a.every((x) => ["crit", "warn", "info"].includes(x.level) && ["today", "intake", "programs", "friday", "notebook"].includes(x.tab) && x.text));
  const lv = a.map((x) => x.level);
  assert.deepEqual(lv, lv.slice().sort((x, y) => ({ crit: 0, warn: 1, info: 2 })[x] - ({ crit: 0, warn: 1, info: 2 })[y]));
  const t = a.map((x) => x.text).join("\n");
  assert.match(t, /전달 기한 넘김: 로그 형식 바꾸기 — 빈칸 3개\(왜·누가 결정 \/ 언제까지와 이유 \/ 확인 방법\)를 되묻거나/);
  assert.match(t, /빈칸 1개: 버튼 추가 — 확인 방법만 되묻습니다\./);
  assert.match(t, /긴급 사후 등록: 장애 대응 — 기술 판단은 보고 도구의 리드가 합니다\. 파트 리드는 대외 소통만 맡습니다\./);
  assert.doesNotMatch(t, /끝난 긴급/);
  assert.match(t, /A: 활성 자리 3개/);
  assert.match(t, /결정 필요: 화면 개편 \(보고 도구\) — 범위 줄일지/);
  assert.doesNotMatch(t, /끝난 일|결정 없음/);
  assert.doesNotMatch(t, /빈칸 있는 정리 중/); // 빈칸 경고는 '접수' 상태만
  assert.equal(a.find((x) => /전달 기한/.test(x.text)).level, "crit");
  assert.equal(a.find((x) => /전달 기한/.test(x.text)).tab, "intake");
  assert.deepEqual(a.find((x) => /전달 기한/.test(x.text)).ref, { coll: "requests", id: "r1" });
  assert.equal(a.find((x) => /자리/.test(x.text)).tab, "programs");
  assert.equal(a.find((x) => /결정 필요/.test(x.text)).tab, "friday");
  // 이름 없는 프로그램은 '프로그램 N'
  assert.equal(K.programLabel(st.programs, "pa"), "프로그램 1");
  assert.equal(K.programLabel(st.programs, ""), "미정");
});

test("todayActions: 정리 중 근무일 10일 답 없음 → 보류 제안", () => {
  const mk = (asked) => baseState({ risks: [{ checkedAt: "2026-10-05" }], requests: [{ id: "q", what: "자료 요청", status: "정리 중", receivedAt: asked, askedAt: asked, why: "a", when: "b", check: "c", program: "x" }] });
  // 10/14(수) 10시 되물음 → 10/28(수)은 근무일 10일
  let a = K.todayActions(mk(iso(2026, 10, 14, 10)), NOW, CAL);
  const hit = a.find((x) => /보류로 돌릴지/.test(x.text));
  assert.ok(hit);
  assert.equal(hit.level, "warn");
  assert.equal(hit.tab, "intake");
  assert.match(hit.text, /근무일 10일 답 없음: 자료 요청/);
  // 근무일 9일이면 아직
  a = K.todayActions(mk(iso(2026, 10, 15, 10)), NOW, CAL);
  assert.equal(a.find((x) => /보류로 돌릴지/.test(x.text)), undefined);
  // askedAt이 없으면 제안 없음
  const st = mk(iso(2026, 10, 1, 10)); st.requests[0].askedAt = null;
  assert.equal(K.todayActions(st, NOW, CAL).find((x) => /보류로 돌릴지/.test(x.text)), undefined);
});

test("todayActions: 보류가 두 사이클(기본 14주) 넘으면 종료 안내 제안", () => {
  const mk = (rec, asked) => baseState({ risks: [{ checkedAt: "2026-10-05" }], requests: [{ id: "h", what: "보류 건", status: "보류", receivedAt: rec, askedAt: asked || null }] });
  const now = D(2027, 2, 10, 10);
  // 14주 = 98일 전
  let a = K.todayActions(mk(iso(2026, 11, 4, 10)), now, CAL);
  let hit = a.find((x) => /종료 안내/.test(x.text));
  assert.ok(hit);
  assert.equal(hit.level, "info");
  assert.equal(hit.tab, "intake");
  assert.match(hit.text, /^보류 14주: 보류 건/);
  // 13주면 아직
  a = K.todayActions(mk(iso(2026, 11, 11, 10)), now, CAL);
  assert.equal(a.find((x) => /종료 안내/.test(x.text)), undefined);
  // askedAt이 더 최근이면 그것부터 셈
  a = K.todayActions(mk(iso(2026, 10, 1, 10), iso(2026, 12, 1, 10)), now, CAL);
  assert.equal(a.find((x) => /종료 안내/.test(x.text)), undefined);
  // 사이클 길이를 줄이면(2+1주 → 6주) 일찍 뜸
  const st = mk(iso(2026, 12, 23, 10)); st.settings = K.withDefaults({ cycleWeeks: 2, cooldownWeeks: 1 });
  assert.ok(K.todayActions(st, now, CAL).find((x) => /종료 안내/.test(x.text)));
});

test("todayActions: 이번 달 리스크 점검 안 함(첫째 주 지난 뒤만)", () => {
  const re = /리스크 표를 아직 점검하지/;
  // 2026-11: 첫 월요일 11/2. 11/4는 첫째 주 → 안내 없음
  assert.equal(K.todayActions(baseState(), D(2026, 11, 4, 10), CAL).find((x) => re.test(x.text)), undefined);
  // 11/10은 둘째 주 → 안내
  let hit = K.todayActions(baseState(), D(2026, 11, 10, 10), CAL).find((x) => re.test(x.text));
  assert.equal(hit.level, "warn");
  assert.equal(hit.tab, "notebook");
  // 지난 달에만 점검 → 안내
  assert.ok(K.todayActions(baseState({ risks: [{ checkedAt: "2026-10-30" }] }), D(2026, 11, 10, 10), CAL).find((x) => re.test(x.text)));
  // 이번 달 점검 → 없음
  assert.equal(K.todayActions(baseState({ risks: [{ checkedAt: "2026-10-30" }, { checkedAt: "2026-11-03" }] }), D(2026, 11, 10, 10), CAL).find((x) => re.test(x.text)), undefined);
  // 달 첫날이 첫 월요일 전(11/1 일)이면 안내 없음
  assert.equal(K.todayActions(baseState(), D(2026, 11, 1, 10), CAL).find((x) => re.test(x.text)), undefined);
});

test("todayActions: 사이클 시작 주에 체크 7개가 다 안 됐으면 안내", () => {
  const re = /사이클 1 시작 주: 시작 체크/;
  const risks = [{ checkedAt: "2026-11-02" }];
  let a = K.todayActions(baseState({ risks }), D(2026, 11, 3, 10), CAL);
  let hit = a.find((x) => re.test(x.text));
  assert.equal(hit.level, "warn");
  assert.equal(hit.tab, "programs");
  assert.match(hit.text, /0\/7/);
  const half = { id: "2026-11-02", checks: { 0: true, 1: true, 2: true } };
  assert.match(K.todayActions(baseState({ risks, cycles: [half] }), D(2026, 11, 3, 10), CAL).find((x) => re.test(x.text)).text, /3\/7/);
  const full = { id: "2026-11-02", checks: { 0: true, 1: true, 2: true, 3: true, 4: true, 5: true, 6: true } };
  assert.equal(K.todayActions(baseState({ risks, cycles: [full] }), D(2026, 11, 3, 10), CAL).find((x) => re.test(x.text)), undefined);
  // 사이클 둘째 주는 안내 없음
  assert.equal(K.todayActions(baseState({ risks }), D(2026, 11, 10, 10), CAL).find((x) => /시작 체크/.test(x.text)), undefined);
  // 아무것도 없으면 빈 배열, state가 비어도 죽지 않음
  assert.deepEqual(K.todayActions(baseState({ risks: [{ checkedAt: "2026-10-01" }] }), D(2026, 10, 21, 10), CAL), []);
  assert.ok(Array.isArray(K.todayActions({}, D(2026, 11, 10, 10))));
});

test("askText: 빈칸만 묻고, 사람 이름 없이 일반 문장", () => {
  const r = { what: "로그 형식 바꾸기", why: "팀장 지시", when: "", check: "", program: "p1" };
  const s = K.askText(r, "프로그램 1");
  assert.match(s, /"로그 형식 바꾸기" \(프로그램 1\) 요청을 접수했습니다/);
  assert.match(s, /2가지만 여쭙니다/);
  assert.match(s, /1\) 언제까지 필요한지와 그 이유\n2\) 끝났다는 것을 어떻게 확인하면 되는지/);
  assert.doesNotMatch(s, /왜 필요한지/);
  assert.match(s, /근무일 2일 안에 담당 리드에게 넘깁니다/);
  assert.doesNotMatch(s, /회신/); // 약속은 '넘기기' 하나뿐(날짜는 리드가 가늠한 뒤)
  const none = K.askText({ what: "x", why: "a", when: "b", check: "c", program: "p" }, "");
  assert.match(none, /빈칸이 없어/);
  assert.match(K.askText({ raw: "원문 첫 줄\n둘째 줄" }), /"원문 첫 줄"/);
});

test("handoffText: 결정된 것 / 열린 것 / 요청자 세 칸", () => {
  const r = { what: "버튼 추가", why: "운영 편의", when: "11월 말, 점검 일정", check: "", program: "p1", size: "S", from: "다른 파트 개발자", raw: "버튼 하나\n추가해 주세요", est: null, urgent: false };
  const s = K.handoffText(r, "프로그램 2");
  const L = s.split("\n");
  assert.equal(L[0], "[접수 넘김] 버튼 추가 — 프로그램 2");
  assert.equal(L[1], "1) 결정된 것: 무엇: 버튼 추가 / 왜·누가 결정: 운영 편의 / 언제까지: 11월 말, 점검 일정 / 크기: S(반나절 이하)");
  assert.equal(L[2], "2) 열린 것: 확인 방법 / 리드 추정(최소·보통·최대)");
  assert.equal(L[3], "3) 요청자: 다른 파트 개발자");
  assert.equal(L[4], "요청 원문: 버튼 하나 추가해 주세요");
  const done = K.handoffText(Object.assign({}, r, { check: "시연", est: { min: "1일", mid: "2일", max: "4일" }, urgent: true }), "프로그램 2");
  assert.match(done, /2\) 열린 것: 없음/);
  assert.match(done, /추정: 최소 1일 · 보통 2일 · 최대 4일/);
  assert.match(done, /\(긴급·사후 등록\)/);
});

test("fridayDraft: 일반 제목, 위험·결정·접수 현황·흡수, 끝난 항목 제외, 마지막 줄", () => {
  const now = D(2026, 10, 30, 14); // 금
  const st = baseState({
    programs: [prog("p1", "p1", "", true, false, { order: 1, name: "" }), prog("p2", "p2", "", true, false, { order: 2, name: "보고 도구" }), prog("p3", "", "", false, false, { order: 3 })],
    mapitems: [
      { id: "m1", item: "화면 개편", program: "p2", risk: "일정 지연", decide: "범위 줄일지", done: false },
      { id: "m2", item: "로그 정리", program: "p1", risk: "", decide: "", done: false },
      { id: "m3", item: "끝난 일", program: "p1", risk: "숨길 위험", decide: "숨길 결정", done: true },
      { id: "m4", item: "공통 모듈", program: "", risk: "검토\n늦음", decide: "", done: false }
    ],
    requests: [
      { status: "접수", receivedAt: iso(2026, 10, 29, 9) }, // 대기, 새(기한 월 09시)
      { status: "정리 중", receivedAt: iso(2026, 10, 22, 9) }, // 대기, 기한 넘김
      { status: "완료", receivedAt: iso(2026, 10, 20, 9), closedAt: iso(2026, 10, 29, 9) },
      { status: "완료", receivedAt: iso(2026, 10, 1, 9), closedAt: iso(2026, 10, 2, 9) },
      { status: "진행", absorbed: true, absorbedAt: iso(2026, 10, 27, 9), receivedAt: iso(2026, 10, 27, 9) },
      { status: "완료", absorbed: true, absorbedAt: iso(2026, 10, 20, 9), receivedAt: iso(2026, 10, 20, 9) }
    ]
  });
  const s = K.fridayDraft(st, now);
  const L = s.split("\n");
  assert.equal(L[0], "주간 보고 (2026-10-30)");
  assert.ok(s.includes("1. 위험 (진행 지도 위험 칸)\n- 화면 개편 (보고 도구): 일정 지연\n- 공통 모듈: 검토 늦음\n"));
  assert.ok(s.includes("2. 결정 필요\n- 화면 개편 (보고 도구): 범위 줄일지\n"));
  assert.doesNotMatch(s, /숨길|끝난 일|로그 정리/);
  assert.ok(s.includes("- 접수 대기 2건 · 전달 2일 초과 1건 · 이번 주 새 요청 2건 · 이번 주 완료 1건"));
  assert.ok(s.includes("- 활성 프로그램 2개"));
  assert.ok(s.includes("4. 이번 주 흡수 건수: 1건"));
  assert.equal(L[L.length - 1], "개인별 진행률·업무량은 적지 않습니다.");
  // opts.programName과 opts.map
  const s2 = K.fridayDraft(st, now, { programName: (id) => "P-" + id, map: [{ item: "새 항목", program: "p9", risk: "r", decide: "", done: false }] });
  assert.match(s2, /- 새 항목 \(P-p9\): r/);
  assert.match(s2, /2\. 결정 필요\n- 없음/);
  // 빈 state
  const e = K.fridayDraft({}, now);
  assert.match(e, /1\. 위험 \(진행 지도 위험 칸\)\n- 없음/);
  assert.match(e, /접수 대기 0건/);
  assert.deepEqual(K.fridayCounts(st, now, CAL), { waiting: 2, over: 1, newThisWeek: 2, doneThisWeek: 1, absorbed: 1, active: 2 });
});

test("tableHTML: 이스케이프와 줄바꿈", () => {
  const h = K.tableHTML(["항목", "메모<b>"], [["a & b", "<script>x</script>"], ["줄1\n줄2", "\"q\" 'q'"], [null, 3]]);
  assert.equal(h, "<table><thead><tr><th>항목</th><th>메모&lt;b&gt;</th></tr></thead><tbody>" +
    "<tr><td>a &amp; b</td><td>&lt;script&gt;x&lt;/script&gt;</td></tr>" +
    "<tr><td>줄1<br>줄2</td><td>&quot;q&quot; &#39;q&#39;</td></tr>" +
    "<tr><td></td><td>3</td></tr></tbody></table>");
  assert.doesNotMatch(h, /<script>/);
  assert.equal(K.tableHTML([], []), "<table><thead><tr></tr></thead><tbody></tbody></table>");
});

test("tableWiki: ||제목|| / |칸|, 칸 안의 | 와 줄바꿈 이스케이프", () => {
  const w = K.tableWiki(["항목", "결정"], [["a|b", "줄1\n줄2"], ["", "[링크] {코드}"], ["끝 줄바꿈\r\n", 0]]);
  assert.equal(w, [
    "||항목||결정||",
    "|a\\|b|줄1 \\\\ 줄2|",
    "| |\\[링크\\] \\{코드\\}|",
    "|끝 줄바꿈|0|"
  ].join("\n"));
  // 각 행의 칸 구분자 수가 같다(이스케이프된 | 제외)
  w.split("\n").slice(1).forEach((row) => assert.equal(row.replace(/\\\|/g, "").split("|").length, 4));
});

test("weekSum: 이번 주 kind별 합계와 총합", () => {
  const T = [
    { date: "2026-10-26", kind: "접수·명확화·회신", hours: 1.5 },
    { date: "2026-10-27", kind: "접수·명확화·회신", hours: 2 },
    { date: "2026-10-28", kind: "멘토링", hours: 0.1 },
    { date: "2026-10-28", kind: "멘토링", hours: 0.2 },
    { date: "2026-11-01", kind: "", hours: 1 }, // 일요일 → 이번 주, 기타
    { date: "2026-10-25", kind: "멘토링", hours: 5 }, // 지난 주
    { date: "2026-10-27", kind: "통합 설계", hours: "x" },
    { date: "", kind: "기타", hours: 1 }
  ];
  const s = K.weekSum(T, NOW);
  assert.equal(s.week, "2026-10-26");
  assert.deepEqual(s.byKind, { "접수·명확화·회신": 3.5, "멘토링": 0.3, "기타": 1 });
  assert.equal(s.total, 4.8);
  assert.equal(s.intake, 3.5);
  assert.equal(s.intakeOver, true);
  const last = K.weekSum(T, K.addDays(NOW, -7));
  assert.equal(last.week, "2026-10-19");
  assert.equal(last.total, 5);
  assert.equal(last.intakeOver, false);
  assert.deepEqual(K.weekSum(null, NOW), { week: "2026-10-26", byKind: {}, total: 0, intake: 0, intakeOver: false });
});

test("core.js: eval·new Function을 쓰지 않는다(CSP)", () => {
  const src = require("node:fs").readFileSync(require("node:path").join(__dirname, "..", "core.js"), "utf8");
  assert.doesNotMatch(src, /\beval\s*\(|new\s+Function\s*\(|document\.|window\.document/);
});


/* ======================================================================
 * 수정 확인(검토 지적)
 * ====================================================================== */

test("정리 주 0을 저장해도 사이클 계산·라벨·cycleId가 모두 1주로 같다", () => {
  const st0 = K.withDefaults({ startDate: "2026-10-19", cycleStartWeek: 2, cycleWeeks: 6, cooldownWeeks: 0 });
  assert.equal(st0.cooldownWeeks, 1);
  assert.equal(K.cyc(st0).cd, 1);
  assert.deepEqual(K.cycleOf(8, st0), K.cycleOf(8, ST));
  assert.equal(K.cycleId(D(2026, 12, 16, 10), st0), K.cycleId(D(2026, 12, 16, 10), ST));
});

test("todayActions: 보류 기간은 heldAt(보류로 바꾼 때)부터 센다", () => {
  const mk = (r) => baseState({ risks: [{ checkedAt: "2026-10-05" }], requests: [Object.assign({ id: "h", what: "보류 건", status: "보류" }, r)] });
  const now = D(2026, 10, 22, 10);
  // 오래전에 받았어도 오늘 보류로 돌렸으면 제안 없음
  let a = K.todayActions(mk({ receivedAt: iso(2026, 6, 1, 10), heldAt: iso(2026, 10, 22, 9) }), now, CAL);
  assert.equal(a.find((x) => /종료 안내/.test(x.text)), undefined);
  // heldAt부터 14주 지나면 제안, N은 heldAt부터
  a = K.todayActions(mk({ receivedAt: iso(2026, 1, 5, 10), heldAt: iso(2026, 7, 15, 9) }), now, CAL);
  const hit = a.find((x) => /종료 안내/.test(x.text));
  assert.ok(hit);
  assert.match(hit.text, /^보류 14주: 보류 건/);
  // heldAt이 없는 옛 데이터는 받은 때부터(지금처럼)
  a = K.todayActions(mk({ receivedAt: iso(2026, 6, 1, 10) }), now, CAL);
  assert.ok(a.find((x) => /종료 안내/.test(x.text)));
});

test("todayActions: 긴급 사후 등록 안내는 프로그램이 비었거나 이번 주 것만, 주어를 밝힌다", () => {
  const mk = (r) => baseState({ risks: [{ checkedAt: "2026-10-05" }], requests: [Object.assign({ id: "u", what: "장비 멈춤", status: "진행", urgent: true }, r)] });
  let a = K.todayActions(mk({ program: "", receivedAt: iso(2026, 10, 1, 10) }), NOW, CAL);
  let hit = a.find((x) => /긴급 사후 등록/.test(x.text));
  assert.ok(hit);
  assert.match(hit.text, /그 프로그램의 리드가 합니다\. 파트 리드는 대외 소통만 맡습니다\. 어느 프로그램인지 채웁니다\./);
  assert.doesNotMatch(hit.text, /미정/);
  // 프로그램을 채웠고 지난주 이전 것이면 목록에서 빠짐
  a = K.todayActions(mk({ program: "x", receivedAt: iso(2026, 10, 1, 10) }), NOW, CAL);
  assert.equal(a.find((x) => /긴급 사후 등록/.test(x.text)), undefined);
  // 이번 주 것은 보임
  a = K.todayActions(mk({ program: "x", receivedAt: iso(2026, 10, 27, 10) }), NOW, CAL);
  assert.ok(a.find((x) => /긴급 사후 등록/.test(x.text)));
});

test("blkClock: 접수 시계와 같은 근무일 말투, 넘기면 '넘김 · …까지였습니다'", () => {
  const b = { kind: "b", receivedAt: iso(2026, 10, 23, 10) }; // 금 10시
  assert.equal(K.blkClock(b, D(2026, 10, 23, 15), CAL).text, "근무일 2일 중 0일 지남 · 화 10/27 10:00까지");
  assert.equal(K.blkClock(b, D(2026, 10, 26, 15), CAL).text, "근무일 2일 중 1일 지남 · 화 10/27 10:00까지");
  const late = K.blkClock(b, D(2026, 10, 27, 11), CAL);
  assert.equal(late.state, "over");
  assert.equal(late.text, "근무일 2일 넘김 · 화 10/27 10:00까지였습니다");
  assert.doesNotMatch(late.text, /시간/);
  const c = { kind: "c", receivedAt: iso(2026, 10, 22, 11) };
  assert.equal(K.blkClock(c, D(2026, 10, 22, 12), CAL).text, "근무일 1일 중 0일 지남 · 금 10/23 11:00까지");
  const p = { kind: "b", receivedAt: iso(2026, 10, 19, 10), pausedAt: iso(2026, 10, 20, 10), pausedDays: 0 };
  assert.equal(K.blkClock(p, D(2026, 10, 22, 10), CAL).text, "시계 멈춤 · 멈춘 근무일 2일");
});

test("tableWiki: 칸 안의 백슬래시가 열 구분자를 깨지 않는다", () => {
  const w = K.tableWiki(["경로", "비고"], [["C:\\work\\", "끝"], ["x\\\\y", "a\\|b"]]);
  assert.equal(w, ["||경로||비고||", "|C:&#92;work&#92;|끝|", "|x&#92;&#92;y|a&#92;\\|b|"].join("\n"));
  // 행마다 칸 구분자 수가 같고, 줄바꿈(\\)이 생기지 않는다
  w.split("\n").slice(1).forEach((row) => {
    assert.equal(row.replace(/\\\|/g, "").split("|").length, 4);
    assert.doesNotMatch(row, /\\\\/);
  });
});

/* ---------- 요청 접수함(한 기능판) ---------- */
test("intakeView: 열린 것은 시계 넘김 → 오늘 기한 → 여유 → 시계 없음, 끝난 것은 done 보기에만", () => {
  const c = K.cal("");
  const now = D(2026, 11, 4, 15); // 수
  const over = { id: "over", status: "접수", receivedAt: D(2026, 10, 29, 10).toISOString() };   // 목 → 월 10시 기한
  const soon = { id: "soon", status: "접수", receivedAt: D(2026, 11, 2, 16).toISOString() };    // 월 16시 → 수 16시
  const ok = { id: "ok", status: "정리 중", receivedAt: D(2026, 11, 4, 9).toISOString() };     // 수 → 금
  const urgent = { id: "urg", status: "진행", urgent: true, receivedAt: D(2026, 11, 4, 8).toISOString() };
  const fwd = { id: "fwd", status: "리드 전달", forwardedAt: D(2026, 11, 3).toISOString(), receivedAt: D(2026, 11, 1).toISOString() };
  const done = { id: "done", status: "완료", receivedAt: D(2026, 11, 3).toISOString() };
  const all = [done, ok, fwd, urgent, soon, over];
  assert.deepEqual(K.intakeView(all, "open", now, c).map((r) => r.id), ["over", "soon", "ok", "urg", "fwd"]);
  assert.deepEqual(K.intakeView(all, "done", now, c).map((r) => r.id), ["done"]);
  assert.equal(K.intakeView(all, "all", now, c).length, 6);
  assert.equal(K.intakeView(all, "all", now, c).slice(-1)[0].id, "done");
  assert.deepEqual(K.intakeView([null, 3, "x"], "all", now, c), []);
});

test("intakeCounts: 열린 요청·기한 넘김·빈칸 있는 접수", () => {
  const c = K.cal("");
  const now = D(2026, 11, 4, 15);
  const reqs = [
    { status: "접수", receivedAt: D(2026, 10, 29, 10).toISOString(), what: "a" },                              // 넘김 + 빈칸
    { status: "정리 중", receivedAt: D(2026, 11, 4, 9).toISOString(), what: "b", why: "w", when: "x", check: "y", program: "p" }, // 빈칸 없음
    { status: "진행", urgent: true, receivedAt: D(2026, 11, 4, 8).toISOString() },                            // 긴급: 빈칸 셈에서 뺌
    { status: "완료", receivedAt: D(2026, 10, 1).toISOString() },
  ];
  assert.deepEqual(K.intakeCounts(reqs, now, c), { open: 3, over: 1, blanks: 1, news: 0 });
});

test("statusPatch: 리드 전달·끝남·보류 시각을 처음 한 번만 적고, 되돌리면 지운다", () => {
  const now = "2026-11-04T06:00:00.000Z";
  assert.deepEqual(K.statusPatch({ status: "정리 중" }, "리드 전달", now), { status: "리드 전달", statusAt: now, forwardedAt: now });
  assert.deepEqual(K.statusPatch({ status: "리드 전달", forwardedAt: "x", statusAt: "x" }, "리드 전달", now), { status: "리드 전달" });
  assert.deepEqual(K.statusPatch({ status: "진행", statusAt: "x" }, "완료", now), { status: "완료", statusAt: now, closedAt: now });
  assert.deepEqual(K.statusPatch({ status: "완료", closedAt: "x" }, "진행", now), { status: "진행", statusAt: now, closedAt: null, reopenedAt: now });
  assert.deepEqual(K.statusPatch({ status: "정리 중" }, "보류", now), { status: "보류", statusAt: now, heldAt: now });
  assert.deepEqual(K.statusPatch({ status: "보류", heldAt: "x" }, "정리 중", now), { status: "정리 중", statusAt: now, heldAt: null });
  // 상태가 없던 옛 요청은 '접수'로 봅니다. statusAt이 없으면 같은 단계라도 적어 둡니다.
  assert.deepEqual(K.statusPatch({}, "접수", now), { status: "접수", statusAt: now });
});

test("parseAskTemplate: 요청자가 채워 보낸 양식을 칸으로 나눈다", () => {
  const ps = [{ id: "p1", name: "주문 처리 서비스" }, { id: "p2", name: "로그 수집기" }, { id: "p3", name: "쉬는 것", active: false }];
  const filled = K.templateText()
    .replace("무엇(한 줄): ", "무엇(한 줄): 로그 내보내기 버튼")
    .replace("왜 필요하고 누가 정했나요: ", "왜 필요하고 누가 정했나요: -")
    .replace("언제까지, 그 날짜인 이유: ", "언제까지, 그 날짜인 이유: 11/20, 시연")
    .replace("어느 프로그램: ", "어느 프로그램: 로그수집기")
    .replace("보내는 분: ", "보내는 분: 다른 파트 개발자")
    .replace("자세히: ", "자세히: 첫 줄\n둘째 줄\n\n셋째 줄");
  assert.deepEqual(K.parseAskTemplate(filled, ps), {
    what: "로그 내보내기 버튼", why: "", when: "11/20, 시연", check: "", from: "다른 파트 개발자", program: "p2",
    raw: "첫 줄\n둘째 줄\n\n셋째 줄", labeled: 7,
  });
  // 손으로 쓴 양식: 글머리·전각 콜론·여러 줄 값, 모르는 프로그램 이름은 자세히에 남긴다
  const hand = "안녕하세요\n- 무엇 ： 배포 스크립트 고치기\n- 기한: 금요일\n  (점검 일정 때문)\n- 확인: 점검 때 한 번에 배포\n- 시스템: 새 시스템\n- 쉬는 것";
  const r = K.parseAskTemplate(hand, ps);
  assert.equal(r.what, "배포 스크립트 고치기");
  assert.equal(r.when, "금요일 (점검 일정 때문)");
  assert.equal(r.check, "점검 때 한 번에 배포");
  assert.equal(r.program, "");
  assert.match(r.raw, /^안녕하세요\n/);
  assert.match(r.raw, /어느 프로그램\(요청자가 적은 것\): 새 시스템/);
  // 양식이 아니면 첫 줄이 '무엇', 전체가 '자세히'
  assert.deepEqual(K.parseAskTemplate("10:30 회의 전에\n로그 버튼 좀 만들어 주세요", ps), {
    what: "10:30 회의 전에", why: "", when: "", check: "", from: "", program: "", raw: "10:30 회의 전에\n로그 버튼 좀 만들어 주세요", labeled: 0,
  });
  assert.equal(K.parseAskTemplate("", ps).what, "");
});

test("matchProgram: 같은 이름, 띄어쓰기 무시, 하나만 겹칠 때만", () => {
  const ps = [{ id: "a", name: "주문 처리 서비스" }, { id: "b", name: "주문 화면" }, { id: "c", name: "쉬는 것", active: false }];
  assert.equal(K.matchProgram(ps, "주문처리서비스"), "a");
  assert.equal(K.matchProgram(ps, "처리 서비스"), "a");
  assert.equal(K.matchProgram(ps, "주문"), "");
  assert.equal(K.matchProgram(ps, "쉬는 것"), "");
  assert.equal(K.matchProgram(ps, ""), "");
});

test("요청 창구 문장: 되묻기에 링크, 링크 안내, 진행 링크", () => {
  const link = K.askLink("http://10.0.0.5:8180/", "abcdefghjk");
  assert.equal(link, "http://10.0.0.5:8180/r/abcdefghjk");
  assert.equal(K.askLink("", "abcdefghjk"), "");
  const ask = K.askText({ what: "버튼", why: "w" }, "로그 수집기", link);
  assert.match(ask, /2가지만 여쭙니다/); // 링크가 있으면 어느 프로그램은 묻지 않음(요청자 화면과 같게)
  assert.doesNotMatch(ask, /어느 프로그램/);
  assert.match(K.askText({ what: "버튼", why: "w" }, ""), /3가지만 여쭙니다/); // 링크 없이 메신저로 물을 때는 그대로
  assert.ok(ask.endsWith("이 링크에서 바로 채워 주시면 됩니다(진행 상황도 여기서 보입니다): " + link));
  assert.ok(!K.askText({ what: "버튼" }, "").includes("링크"));
  const full = K.askText({ what: "버튼", why: "w", when: "x", check: "y", program: "p" }, "로그 수집기", link);
  assert.ok(full.endsWith("진행 상황은 이 링크에서 보실 수 있습니다: " + link));
  assert.ok(K.inviteText("http://10.0.0.5:8180/").endsWith("\nhttp://10.0.0.5:8180/"));
  assert.match(K.inviteText("u"), /^번거로우시겠지만 .*근무일 2일 안에 담당 리드에게 넘기고/);
  assert.equal(K.statusLinkText({ what: "버튼" }, link), "\"버튼\" 요청의 진행 상황은 이 링크에서 보실 수 있습니다: " + link);
});

test("askCodeFrom: 서버와 같은 글자 10개", () => {
  const c = K.askCodeFrom([0, 1, 30, 31, 255, 7, 8, 9, 10, 11]);
  assert.ok(K.ASK_CODE_RE.test(c), c);
  assert.equal(c.length, 10);
  assert.equal(c.slice(0, 4), "23z2"); // 0→2, 1→3, 30→z, 31→2(31로 나눈 나머지)
  assert.ok(!K.ASK_CODE_RE.test("abcdefghj"));
  assert.ok(!K.ASK_CODE_RE.test("abcdefghj0"));
});

test("askStage: 요청자에게 보이는 단계", () => {
  assert.equal(K.askStage("접수").step, 0);
  assert.equal(K.askStage(undefined).label, "받음");
  assert.equal(K.askStage("정리 중").step, 1);
  assert.equal(K.askStage("리드 전달").label, "리드에게 넘김");
  assert.equal(K.askStage("완료").step, 4);
  assert.equal(K.askStage("보류").step, -1);
  assert.equal(K.askStage("보류").label, "보류");
  assert.equal(K.askStage("종료").label, "진행 안 함");
  assert.equal(K.ASK_STEPS.length, 5);
});

/* ---------- 요청자에게 보이는 진행 ---------- */
const at = (y, m, d, h = 10, mi = 0) => new Date(y, m - 1, d, h, mi).toISOString();

test("askNow: 빈칸이 있으면 요청자 차례이고, 늦음을 말하지 않는다", () => {
  const c = K.cal("");
  const v = { status: "정리 중", receivedAt: at(2026, 10, 26), askedAt: at(2026, 10, 27), what: "x", missing: ["when", "check"] };
  const n = K.askNow(v, D(2026, 11, 20, 10), c); // 받은 지 한참 지났어도
  assert.equal(n.who, "me");
  assert.equal(n.whoLabel, "요청하신 분 차례");
  assert.equal(n.label, "답을 기다림");
  assert.equal(n.late, false);
  assert.equal(n.due, null);
  assert.match(n.lines[0], /^10\/27\(화\)에 담당자가 빈칸 2개를 여쭸습니다\(언제까지와 이유 \/ 확인 방법\)\.$/);
  assert.match(n.lines[2], /^11\/10\(화\)까지 답이 없으면 보류해 둡니다\.$/); // 되물은 때부터 근무일 10일
  // 어느 프로그램인지는 요청자의 빈칸이 아닙니다.
  assert.equal(K.askNow({ status: "접수", receivedAt: at(2026, 11, 2), why: "a", when: "b", check: "c" }, D(2026, 11, 2, 11), c).who, "desk");
});

test("askNow: 담당자 차례면 넘길 날(날짜만)을 말하고, 마지막 빈칸이 채워진 때부터 센다", () => {
  const c = K.cal("");
  const base = { status: "정리 중", receivedAt: at(2026, 11, 2, 16, 37), why: "a", when: "b", check: "c", missing: [] };
  let n = K.askNow(base, D(2026, 11, 3, 9), c);
  assert.equal(n.who, "desk");
  assert.equal(n.lines[0], "11/4(수)까지 담당 리드에게 넘깁니다.");
  assert.equal(n.late, false);
  assert.equal(n.lines[1], K.DATE_PROMISE);
  // 요청자가 목요일에 빈칸을 채웠으면 그때부터 근무일 2일(다음 주 월요일)
  n = K.askNow(Object.assign({}, base, { filledAt: at(2026, 11, 5, 14) }), D(2026, 11, 6, 9), c);
  assert.equal(n.lines[0], "11/9(월)까지 담당 리드에게 넘깁니다.");
  // 날이 지나면 사실만 말합니다(색·사과 없음).
  n = K.askNow(base, D(2026, 11, 5, 9), c);
  assert.equal(n.late, true);
  assert.equal(n.lines[0], "넘기기로 한 날(11/4(수))이 지났습니다. 넘기면 이 화면에 바로 보입니다.");
  // 같은 날 안에서는 늦음이 아닙니다(시각까지 따지지 않음).
  assert.equal(K.askNow(base, D(2026, 11, 4, 23), c).late, false);
  // 쉬는 날을 뺍니다.
  n = K.askNow(base, D(2026, 11, 3, 9), K.cal("2026-11-04 쉬는 날"));
  assert.equal(n.lines[0], "11/5(목)까지 담당 리드에게 넘깁니다.");
});

test("askNow: 넘긴 뒤·작업 중·끝남·보류·진행 안 함·취소", () => {
  const c = K.cal("");
  const now = D(2026, 11, 10, 10);
  let n = K.askNow({ status: "리드 전달", forwardedAt: at(2026, 11, 3), programName: "로그 수집기" }, now, c);
  assert.deepEqual([n.who, n.label, n.step], ["lead", "리드에게 넘김", 2]);
  assert.equal(n.lines[0], "11/3(화)에 로그 수집기 담당 리드에게 넘겼습니다.");
  assert.match(n.lines[1], /담당 리드가 작업량을 가늠한 뒤 날짜를/);
  n = K.askNow({ status: "진행", statusAt: at(2026, 11, 5), forwardedAt: at(2026, 11, 3), eta: "2026-11-20" }, now, c);
  assert.equal(n.lines[0], "11/5(목)부터 담당 리드가 작업하고 있습니다.");
  assert.equal(n.lines[1], "끝날 날짜: 11/20(금)쯤 — 담당 리드가 가늠한 날이라 바뀔 수 있습니다.");
  // 알린 날짜가 지나도 요청자 화면은 '지남'을 말하지 않습니다(담당자만 봄).
  n = K.askNow({ status: "진행", eta: "2026-11-01" }, now, c);
  assert.ok(!n.lines.join(" ").includes("지남") && !n.lines.join(" ").includes("지났"));
  n = K.askNow({ status: "완료", closedAt: at(2026, 11, 9), check: "파일이 받아짐" }, now, c);
  assert.deepEqual([n.who, n.step], ["me", 4]);
  assert.equal(n.lines[1], "요청하실 때 적으신 확인 방법: 파일이 받아짐");
  n = K.askNow({ status: "완료", closedAt: at(2026, 11, 9), doneOkAt: at(2026, 11, 9, 15) }, now, c);
  assert.equal(n.who, "");
  assert.equal(n.lines[1], "11/9(월)에 확인해 주셨습니다.");
  n = K.askNow({ status: "보류", heldAt: at(2026, 11, 6), missing: ["why"], editable: true, reply: K.HOLD_REASONS[0] }, now, c);
  assert.deepEqual([n.who, n.label, n.step], ["me", "보류", -1]);
  assert.equal(n.lines[1], "빈칸을 채워 주시면 담당자가 확인해 다시 이어 갑니다.");
  // 다른 까닭의 보류라면 빈칸이 있어도 요청자 차례가 아닙니다.
  assert.equal(K.askNow({ status: "보류", heldAt: at(2026, 11, 6), missing: ["why"], editable: true, reply: K.HOLD_REASONS[2] }, now, c).who, "desk");
  assert.equal(K.askNow({ status: "보류", heldAt: at(2026, 11, 6), missing: ["why"], editable: false, reply: K.HOLD_REASONS[0] }, now, c).who, "desk");
  assert.equal(K.askNow({ status: "보류", heldAt: at(2026, 11, 6), forwardedAt: at(2026, 11, 3), missing: [] }, now, c).who, "desk");
  n = K.askNow({ status: "종료", closedAt: at(2026, 11, 6) }, now, c);
  assert.deepEqual([n.who, n.label], ["", "진행 안 함"]);
  n = K.askNow({ status: "접수", withdrawnAt: at(2026, 11, 6) }, now, c);
  assert.deepEqual([n.label, n.step], ["취소함", -1]);
  n = K.askNow({ status: "리드 전달", forwardedAt: at(2026, 11, 3), withdrawnAt: at(2026, 11, 6) }, now, c);
  assert.deepEqual([n.who, n.label, n.step], ["desk", "취소 알림", 2]);
  assert.equal(n.last, at(2026, 11, 6));
});

test("취소(withdrawnAt): 넘기기 전이면 닫힌 것, 넘긴 뒤면 담당자가 닫을 때까지 열림", () => {
  const c = K.cal("");
  const now = D(2026, 11, 4, 15);
  const before = { id: "b", status: "접수", receivedAt: D(2026, 10, 29, 10).toISOString(), withdrawnAt: D(2026, 11, 3).toISOString() };
  const fwd = { id: "f", status: "리드 전달", forwardedAt: D(2026, 11, 2).toISOString(), receivedAt: D(2026, 11, 1).toISOString(), withdrawnAt: D(2026, 11, 3).toISOString() };
  assert.equal(K.isOpenReq(before), false);
  assert.equal(K.reqClock(before, now, c), null); // 기한 넘김으로 세지 않음
  assert.equal(K.isOpenReq(fwd), true);
  assert.equal(K.askNews(fwd).level, "crit");
  assert.deepEqual(K.intakeView([before, fwd], "open", now, c).map((r) => r.id), ["f"]);
  assert.equal(K.fridayCounts({ requests: [before] }, now, c).waiting, 0);
  assert.deepEqual(K.intakeCounts([before, fwd], now, c), { open: 1, over: 0, blanks: 0, news: 2 });
});

test("askNews: 요청자 소식은 하나만, 중요한 것부터, 본 뒤에는 사라진다", () => {
  const t = (d, h = 10) => D(2026, 11, d, h).toISOString();
  assert.equal(K.askNews({ status: "접수" }), null);
  let n = K.askNews({ status: "정리 중", askedAt: t(2), addedAt: t(3), filledAt: t(3) });
  assert.deepEqual([n.kind, n.unseen], ["filled", true]);
  assert.equal(K.askNews({ status: "정리 중", askedAt: t(2), addedAt: t(3), filledAt: t(3), seenAt: t(4) }), null);
  assert.equal(K.askNews({ status: "정리 중", addedAt: t(3), askedAt: t(4) }), null); // 다시 되물었으면 옛 소식
  n = K.askNews({ status: "리드 전달", forwardedAt: t(2), addedAt: t(5), filledAt: t(1), adds: [{ at: t(5), text: "x" }] });
  assert.equal(n.kind, "add");
  assert.equal(K.askNews({ status: "리드 전달", forwardedAt: t(2), addedAt: t(5) }).kind, "add"); // adds 이전의 옛 데이터
  // 끝남 확인으로 addedAt이 바뀌어도 '덧붙임'이 아닙니다(완료로 닫은 뒤 다시 올라오지 않음).
  assert.equal(K.askNews({ status: "완료", closedAt: t(6), doneOkAt: t(5), addedAt: t(5), filledAt: t(1), seenAt: t(2) }), null);
  assert.equal(K.reqAttention({ status: "완료", closedAt: t(6), doneOkAt: t(5), addedAt: t(5), seenAt: t(2) }), false);
  n = K.askNews({ status: "리드 전달", forwardedAt: t(2), doneOkAt: t(5), addedAt: t(5), seenAt: t(6) });
  assert.deepEqual([n.kind, n.level, n.unseen], ["doneok", "good", false]); // 완료로 바꿀 때까지 남음
  n = K.askNews({ status: "완료", closedAt: t(4), doneNoAt: t(5), addedAt: t(5) });
  assert.deepEqual([n.kind, n.level], ["notyet", "crit"]);
  assert.equal(K.reqAttention({ status: "완료", closedAt: t(4), doneNoAt: t(5), addedAt: t(5) }), true); // 끝난 것이라도 열린 보기에 다시
  assert.equal(K.reqAttention({ status: "종료", closedAt: t(4), addedAt: t(5) }), true); // 다시 봐 달라는 덧붙임
  assert.equal(K.reqAttention({ status: "종료", closedAt: t(4), addedAt: t(5), seenAt: t(6) }), false);
  n = K.askNews({ status: "리드 전달", forwardedAt: t(2), withdrawnAt: t(5), doneNoAt: t(4), seenAt: t(6) });
  assert.equal(n.kind, "cancel"); // 넘긴 뒤 취소는 본 뒤에도 닫을 때까지 남음
});

test("요청자 소식 문장: 이유·날짜·덧붙임 전달·보고 한 줄", () => {
  const link = "http://10.0.0.5:8180/r/abcdefghjk";
  assert.equal(K.replyText({ what: "버튼", status: "보류", reply: K.HOLD_REASONS[1] }, link),
    "\"버튼\" 요청은 잠시 보류합니다.\n" + K.HOLD_REASONS[1] + "\n자세한 진행 상황은 이 링크에서 보실 수 있습니다: " + link);
  assert.match(K.replyText({ what: "버튼", status: "종료", reply: "x" }, ""), /^"버튼" 요청은 진행하지 않기로 했습니다\.\nx$/);
  assert.match(K.etaText({ what: "버튼", eta: "2026-11-20" }, link), /^"버튼" 요청은 담당 리드가 가늠해 보니 11\/20\(금\)쯤 끝날 것 같습니다\./);
  const r = { what: "버튼", adds: [{ at: D(2026, 11, 2).toISOString(), text: "전" }, { at: D(2026, 11, 5).toISOString(), text: "날짜\n당겨짐" }, { at: D(2026, 11, 6).toISOString(), text: "다름", kind: "notyet" }, { text: "" }, 3] };
  assert.equal(K.addsText(r, D(2026, 11, 3).toISOString()), "[요청자 덧붙임] 버튼\n- 11/5(목): 날짜 당겨짐\n- 11/6(금) (아직 다름): 다름");
  assert.equal(K.addsText(r, D(2026, 11, 7).toISOString()), "");
  assert.match(K.handoffText(r, "p"), /요청자 덧붙임: 전 \/ 날짜 당겨짐 \/ 다름/);
  assert.equal(K.askReportText({ what: "버튼", status: "리드 전달", forwardedAt: D(2026, 11, 3).toISOString() }, link, D(2026, 11, 4), K.cal("")),
    "[요청 현황] 버튼 — 지금: 리드에게 넘김(11/3(화)) · 날짜: 담당 리드가 가늠한 뒤 정해짐 · " + link);
  // 보류·종료 이유는 탓하는 말이 아니고, 거절만 있는 이유는 없습니다(거절은 팀장과 정한 뒤).
  for (const x of K.HOLD_REASONS.concat(K.CLOSE_REASONS)) assert.doesNotMatch(x, /우선순위가? 낮|불명확|안 됩니다|못 합니다/);
  assert.ok(K.CLOSE_REASONS.some((x) => /팀장과 정한/.test(x)));
});

test("검토 뒤 고침: 리드가 맡은 기준·다시 연 요청·아직 다름·지난 소식", () => {
  const c = K.cal("");
  const t = (d, h = 10) => D(2026, 11, d, h).toISOString();
  // 긴급처럼 리드 전달 없이 진행된 요청을 취소하면 '리드에게 알리고 닫기'(넘기기 전 취소가 아님)
  const urg = { id: "u", status: "진행", urgent: true, receivedAt: t(2), withdrawnAt: t(4) };
  assert.equal(K.isOpenReq(urg), true);
  assert.equal(K.askNews(urg).level, "crit");
  assert.equal(K.askNow(urg, D(2026, 11, 5), c).label, "취소 알림");
  // 리드가 맡은 뒤 취소했는데 완료로 닫았다면: 취소함(요청자에게 할 일을 내밀지 않음)
  assert.deepEqual([K.askNow({ status: "완료", forwardedAt: t(2), withdrawnAt: t(4), closedAt: t(5) }, D(2026, 11, 6), c).label, K.askNow({ status: "완료", forwardedAt: t(2), withdrawnAt: t(4), closedAt: t(5) }, D(2026, 11, 6), c).who], ["취소함", ""]);
  // 다시 연 요청: 그 전의 '끝났어요'는 셈하지 않음
  const re = { status: "진행", forwardedAt: t(2), doneOkAt: t(5), reopenedAt: t(6), statusAt: t(6) };
  assert.equal(K.doneState(re), "");
  assert.equal(K.askNow(re, D(2026, 11, 9), c).who, "lead");
  assert.equal(K.askNews(re), null);
  assert.equal(K.askNow(Object.assign({}, re, { status: "완료", closedAt: t(9) }), D(2026, 11, 10), c).who, "me"); // 다시 확인할 차례
  // 아직 다름은 열어 봐도 남고, 상태를 바꾸거나 남길 말을 새로 적으면 사라짐
  const no = { status: "완료", forwardedAt: t(2), closedAt: t(4), statusAt: t(4), doneNoAt: t(5), addedAt: t(5), adds: [{ at: t(5), text: "다름", kind: "notyet" }], seenAt: t(6) };
  assert.equal(K.askNews(no).kind, "notyet");
  assert.equal(K.askNews(no).unseen, false);
  assert.equal(K.reqAttention(no), true);
  assert.equal(K.askNews(Object.assign({}, no, { reply: "확인해 보니 맞습니다", replyAt: t(7) })), null);
  // 끝남 확인 뒤에 온 덧붙임은 가려지지 않음
  assert.equal(K.askNews({ status: "진행", forwardedAt: t(2), doneOkAt: t(5), seenAt: t(6), addedAt: t(7), adds: [{ at: t(7), text: "하나 더" }] }).kind, "add");
  // 넘기기 전의 빈칸 채움은 넘기거나 닫은 뒤에는 새 소식이 아님
  const old = { status: "완료", askedAt: t(2), filledAt: t(3), addedAt: t(3), forwardedAt: t(4), closedAt: t(9), statusAt: t(9) };
  assert.equal(K.askNews(old), null);
  assert.equal(K.reqAttention(old), false);
  // 열어 둔 요청은 소식을 본 것으로 쳐도 보기에서 빠지지 않음
  const seen = Object.assign({}, no, { id: "n", reply: "x", replyAt: t(7) });
  assert.deepEqual(K.intakeView([seen], "open", D(2026, 11, 10), c).map((r) => r.id), []);
  assert.deepEqual(K.intakeView([seen], "open", D(2026, 11, 10), c, "n").map((r) => r.id), ["n"]);
  // 보고용 한 줄: 보류·진행 안 함·취소에는 날짜를 붙이지 않음
  assert.doesNotMatch(K.askReportText({ what: "x", status: "보류", heldAt: t(4), eta: "2026-11-20" }, "", D(2026, 11, 5), c), /날짜/);
  assert.doesNotMatch(K.askReportText({ what: "x", status: "리드 전달", forwardedAt: t(2), withdrawnAt: t(4) }, "", D(2026, 11, 5), c), /날짜/);
  assert.match(K.askReportText({ what: "x", status: "진행", statusAt: t(3), eta: "2026-11-20" }, "", D(2026, 11, 5), c), /날짜: 11\/20\(금\)쯤/);
});

test("되묻기 기다림: 요청자 화면의 날과 담당자에게 '보류할지' 뜨는 날이 같다", () => {
  const c = K.cal("");
  const asked = D(2026, 11, 9, 20).toISOString(); // 월 20시(근무 시간 뒤)
  const page = K.askNow({ status: "정리 중", receivedAt: asked, askedAt: asked, missing: ["why"] }, D(2026, 11, 10), c).lines[2];
  assert.equal(page, "11/23(월)까지 답이 없으면 보류해 둡니다.");
  const st = { requests: [{ id: "a", what: "x", status: "정리 중", receivedAt: asked, askedAt: asked }] };
  const has = (d) => K.todayActions(st, d, c).some((a) => /답 없음/.test(a.text));
  assert.equal(has(D(2026, 11, 20, 10)), false);
  assert.equal(has(D(2026, 11, 23, 10)), true);
});

test("replyNow: 지금 단계 전에 남긴 말(예: 지난 보류 이유)은 요청자에게 보이지 않는다", () => {
  const t = (d) => D(2026, 11, d, 10).toISOString();
  assert.equal(K.replyNow({ status: "보류", statusAt: t(3), reply: " 이유 ", replyAt: t(3) }), "이유"); // 같은 때 = 지금 것
  assert.equal(K.replyNow({ status: "리드 전달", statusAt: t(5), reply: "지난 보류 이유", replyAt: t(3) }), "");
  assert.equal(K.replyNow({ status: "리드 전달", statusAt: t(5), reply: "맡았습니다", replyAt: t(6) }), "맡았습니다");
  assert.equal(K.replyNow({ reply: "옛 데이터" }), "옛 데이터");
  assert.equal(K.replyNow({ reply: "" }), "");
});

test("etaState: 알린 날짜는 담당자만 본다(지남·내일)", () => {
  const c = K.cal("");
  assert.equal(K.etaState({ status: "진행", eta: "2026-11-05" }, D(2026, 11, 6, 9), c), "past");
  assert.equal(K.etaState({ status: "진행", eta: "2026-11-09" }, D(2026, 11, 6, 9), c), "soon"); // 금 → 다음 근무일 월
  assert.equal(K.etaState({ status: "진행", eta: "2026-11-20" }, D(2026, 11, 6, 9), c), "");
  assert.equal(K.etaState({ status: "완료", eta: "2026-11-05" }, D(2026, 11, 6, 9), c), "");
  assert.equal(K.etaState({ status: "진행", eta: "다음 주" }, D(2026, 11, 6, 9), c), "");
});
