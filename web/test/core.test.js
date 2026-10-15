// 순수 함수 시험: node --test web/test/
// 근무일 시계는 로컬 시간대로 계산하므로 Asia/Seoul로 고정합니다(첫 Date 사용 전에 지정).
process.env.TZ = "Asia/Seoul";
const test = require("node:test");
const assert = require("node:assert/strict");
const K = require("../app.js");

const D = (y, m, d, hh = 0, mm = 0) => new Date(y, m - 1, d, hh, mm);
const fmt = (d) => K.ymd(d) + " " + K.hm(d);

test("stripNames: 호칭이 붙은 이름과 @멘션을 지운다", () => {
  let r = K.stripNames("김철수님이 배포 권한을 요청했어요");
  assert.equal(r.text, "[팀원]이 배포 권한을 요청했어요");
  assert.equal(r.n, 1);
  r = K.stripNames("박영희 대리가 @hong.gd 에게 넘김");
  assert.equal(r.text, "[팀원]가 @[팀원] 에게 넘김");
  assert.equal(r.n, 2);
});

test("stripNames: 직함·일반 명사는 남긴다", () => {
  const r = K.stripNames("팀장님이 고객님께 회신, 디자인팀 님 아님");
  assert.equal(r.n, 0);
  assert.equal(r.text, "팀장님이 고객님께 회신, 디자인팀 님 아님");
});

test("근무일 시계: 금 17시 막힘 → 화 17시", () => {
  const c = K.cal(K.HOLIDAYS_DEFAULT);
  const b = { kind: "b", receivedAt: D(2026, 10, 23, 17).toISOString() };
  assert.equal(fmt(K.dueOf(b, D(2026, 10, 23, 18), c)), "2026-10-27 17:00");
});

test("근무일 시계: 목 11시 헷갈림 → 금 11시", () => {
  const c = K.cal(K.HOLIDAYS_DEFAULT);
  const b = { kind: "c", receivedAt: D(2026, 10, 22, 11).toISOString() };
  assert.equal(fmt(K.dueOf(b, D(2026, 10, 22, 12), c)), "2026-10-23 11:00");
});

test("근무일 시계: 19시 이후는 다음 근무일 09시부터, 08시 이전은 그날 09시부터", () => {
  const c = K.cal("");
  assert.equal(fmt(c.clockStart(D(2026, 10, 23, 20))), "2026-10-26 09:00");
  assert.equal(fmt(c.clockStart(D(2026, 10, 21, 7, 30))), "2026-10-21 09:00");
  assert.equal(fmt(c.clockStart(D(2026, 10, 24, 11))), "2026-10-26 09:00"); // 토요일
});

test("근무일 시계: 쉬는 날(성탄절 2026-12-25 금)을 건너뛴다", () => {
  const c = K.cal(K.HOLIDAYS_DEFAULT);
  const b = { kind: "c", receivedAt: D(2026, 12, 24, 11).toISOString() };
  assert.equal(fmt(K.dueOf(b, D(2026, 12, 24, 12), c)), "2026-12-28 11:00");
  assert.equal(c.isWorkday(D(2026, 12, 25)), false);
});

test("근무일 시계: 멈춘 동안 지난 근무일만큼 기한이 밀리고, 상태가 바뀐다", () => {
  const c = K.cal("");
  const b = { kind: "b", receivedAt: D(2026, 10, 26, 10).toISOString(), pausedAt: D(2026, 10, 27, 10).toISOString(), pausedDays: 0 };
  assert.equal(K.clockState(b, D(2026, 10, 29, 10), c), "paused");
  assert.equal(K.pausedDaysNow(b, D(2026, 10, 29, 10), c), 2);
  assert.equal(fmt(K.dueOf(b, D(2026, 10, 29, 10), c)), "2026-10-30 10:00");
  const open = { kind: "b", receivedAt: D(2026, 10, 26, 10).toISOString() };
  assert.equal(K.clockState(open, D(2026, 10, 27, 9), c), "ok");
  assert.equal(K.clockState(open, D(2026, 10, 28, 9), c), "soon");
  assert.equal(K.clockState(open, D(2026, 10, 28, 11), c), "over");
  assert.equal(K.onTime(Object.assign({}, open, { resolvedAt: D(2026, 10, 28, 9).toISOString() }), c), true);
  assert.equal(K.onTime(Object.assign({}, open, { resolvedAt: D(2026, 10, 28, 11).toISOString() }), c), false);
});

test("사이클: 기본 설정이 결정 메모 날짜와 맞는다", () => {
  const st = { startDate: "2026-10-19", cycleStartWeek: 2, cycleWeeks: 6, cooldownWeeks: 1 };
  assert.equal(K.weekNo(D(2026, 11, 2, 10), st), 2);
  assert.equal(K.cycleOf(2, st).label, "사이클 1 · 1/6주");
  assert.equal(K.cycleOf(8, st).cool, true); // 12-14 주 정리 주
  assert.equal(K.cycleOf(9, st).label, "사이클 2 · 1/6주"); // 12-21
  assert.equal(K.ymd(K.cycleStart(9, st)), "2026-12-21");
  const m = K.markers(st).map((x) => K.ymd(x.d));
  assert.deepEqual(m, ["2026-11-20", "2026-12-11", "2027-01-18", "2027-04-19", "2027-10-18"]);
});

test("줄 나누기: 글머리표·탭·이름을 정리한다", () => {
  const r = K.pasteLines("- 김민수님이 결제 이관 끝냄\n\nPROJ-1\t로그인 개선\t\t진행 중\t\n• [ ] 남은 것\n담당자\t요약");
  assert.deepEqual(r.lines, ["[팀원]이 결제 이관 끝냄", "PROJ-1 · 로그인 개선 · 진행 중", "남은 것", "담당자 · 요약"]);
  assert.equal(r.names, 1);
  assert.equal(r.personCols, true);
});

const FIELDS = {
  done: "결제 이관 끝남\n검색 색인 재구축 끝남",
  risks: "스테이징 DB 권한\n외부 API 지연\n디자인 확정 지연\n네 번째 위험",
  decisions: "검색 범위 줄이기",
  scope: "",
  remaining: "알림 개편\n관리자 화면",
  gap: "디자인 확정 여부"
};

test("틀 조립: 팀판은 6줄 이하, 순서는 끝난 것 → 위험 → 결정 → 빈칸", () => {
  const d = K.buildDrafts(FIELDS, { companyTemplate: "" });
  assert.ok(d.team.length <= 6);
  assert.deepEqual(d.team.map((x) => x.line.split(":")[0]), ["끝남", "끝남", "위험", "위험", "결정 필요", "빈칸 제안"]);
  assert.match(d.team[3].line, /외부 API 지연 · 디자인 확정 지연/); // 위험 3개까지, 넘친 줄은 묶음
  assert.doesNotMatch(JSON.stringify(d.team), /네 번째 위험/);
  assert.equal(d.team[0].source, "끝난 것·바뀐 결정");
  assert.ok(d.notes.some((n) => /앞의 3개/.test(n)));
});

test("틀 조립: 상사 5줄은 정확히 5줄이고 빈 칸은 '입력 없음'", () => {
  const d = K.buildDrafts(FIELDS, {});
  assert.equal(d.boss5.length, 5);
  assert.equal(d.boss5[3].line, "바뀐 범위: 입력 없음");
  assert.equal(d.boss5[1].line, "위험: 스테이징 DB 권한 · 외부 API 지연 · 디자인 확정 지연");
  assert.equal(d.boss5[4].source, "이번 사이클에 남은 범위");
  assert.equal(d.boss5[3].source, ""); // '입력 없음' 줄은 출처 없음
  const empty = K.buildDrafts({}, {});
  assert.equal(empty.boss5.length, 5);
  assert.ok(empty.boss5.every((x) => /입력 없음$/.test(x.line)));
  assert.equal(empty.team.length, 0);
});

test("틀 조립: gaps에 빈 칸·아직 없음·나중에 보낼 것", () => {
  const d = K.buildDrafts(FIELDS, { readNone: ["결정 로그"], laterCount: 2 });
  assert.deepEqual(d.gaps, ["바뀐 범위", "결정 로그 (아직 없음)", "나중에 보낼 것 2건 남음"]);
});

test("회사 양식: 자리표시자 방식", () => {
  const tpl = "1. 금주 실적\n- {{끝난것}}\n2. 이슈: {{위험}}\n3. 결정 요청\n{{ 결정 필요 }}\n4. 빈칸 {{빈칸}}";
  const r = K.fillCompany(tpl, FIELDS);
  assert.equal(r.mode, "placeholder");
  assert.equal(r.text, "1. 금주 실적\n- 결제 이관 끝남\n- 검색 색인 재구축 끝남\n2. 이슈: 스테이징 DB 권한, 외부 API 지연, 디자인 확정 지연\n3. 결정 요청\n검색 범위 줄이기\n4. 빈칸 디자인 확정 여부");
  assert.deepEqual(r.unmatched, ["이번 사이클에 남은 범위"]);
  assert.ok(r.map.length >= 4);
});

test("회사 양식: 제목 방식(기본 양식, 안내 부분은 뺀다)", () => {
  const tpl = "■ 금주 실적\n- \n■ 차주 계획\n- \n■ 이슈·요청\n- \n---안내---\n이건 기본 양식입니다.";
  const r = K.fillCompany(tpl, FIELDS);
  assert.equal(r.mode, "heading");
  assert.equal(r.text, [
    "■ 금주 실적", "- 결제 이관 끝남", "- 검색 색인 재구축 끝남",
    "■ 차주 계획", "- 알림 개편", "- 관리자 화면",
    "■ 이슈·요청", "- 위험: 스테이징 DB 권한", "- 위험: 외부 API 지연", "- 위험: 디자인 확정 지연", "- 결정 필요: 검색 범위 줄이기"
  ].join("\n"));
  assert.deepEqual(r.map.map((m) => m.source), ["끝난 것·바뀐 결정", "이번 사이클에 남은 범위", "위험, 결정이 필요한 것"]);
  assert.doesNotMatch(r.text, /안내|기본 양식/);
});

test("회사 양식: 제목 짝짓기 낱말(완료·한 일·예정·다음·리스크·건의)과 빈 칸", () => {
  const tpl = "[이번 주 한 일]\n[다음 주 예정]\n\n[리스크 및 건의]\n[기타]\n자유롭게";
  const r = K.fillCompany(tpl, { done: "A", remaining: "", risks: "R" });
  assert.equal(r.text, "[이번 주 한 일]\n- A\n[다음 주 예정]\n- 입력 없음\n\n[리스크 및 건의]\n- 위험: R\n[기타]\n자유롭게");
});

test("회사 양식: 짝지을 제목이 없으면 기본 칸으로 채운다", () => {
  const r = K.fillCompany("주간 보고\n작성일:", { done: "A" });
  assert.equal(r.mode, "fallback");
  assert.match(r.text, /^■ 금주 실적\n- A/);
  assert.ok(r.notes[0].includes("기본 칸"));
});

test("회사 양식: 제목 아래 기존 설명 줄은 그대로 둔다", () => {
  const r = K.fillCompany("금주 실적:\n(완료 항목 위주)\n• \n차주 계획:\n• ", { done: "A\nB", remaining: "C" });
  assert.equal(r.text, "금주 실적:\n(완료 항목 위주)\n• A\n• B\n차주 계획:\n• C");
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
});

test("사내 AI 응답 정리: boss5는 5줄로 맞추고, 빈 값은 틀 초안으로", () => {
  const draft = K.buildDrafts(FIELDS, {});
  const p = K.normalizePolish({ team: ["a", { line: "b", source: "위험" }], boss5: ["1", "2"], company: "" }, draft);
  assert.equal(p.team.length, 2);
  assert.equal(p.boss5.length, 5);
  assert.equal(p.boss5[4].line, "입력 없음");
  assert.equal(p.company, draft.company);
  assert.deepEqual(p.gaps, draft.gaps);
  assert.equal(K.plain(p.team), "1. a\n2. b");
});
