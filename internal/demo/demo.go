// Package demo fills a store with example data for demo mode (--demo).
//
// Every date is computed from "now", so the example always shows the same
// situations whenever it is opened: one request past its 2-workday handoff
// clock, one request with blank fields, one waiting long enough that the
// notebook suggests putting it on hold, one person over the slot cap, an open
// blocker past its clock, and so on. All names are generic: no company,
// person or system names (the image is public).
package demo

import (
	"context"
	"fmt"
	"time"

	"github.com/jameskwon07/manager-notebook/internal/store"
)

type doc = store.Doc

// Seed writes the example data. It expects an empty store.
func Seed(ctx context.Context, st *store.Store, now time.Time) error {
	now = now.In(time.Local)
	day := func(daysAgo, hh, mm int) time.Time {
		d := now.AddDate(0, 0, -daysAgo)
		return time.Date(d.Year(), d.Month(), d.Day(), hh, mm, 0, 0, time.Local)
	}
	iso := func(t time.Time) string { return t.UTC().Format(time.RFC3339) }
	ymd := func(t time.Time) string { return t.Format("2006-01-02") }
	monday := func(t time.Time) time.Time {
		off := (int(t.Weekday()) + 6) % 7
		d := t.AddDate(0, 0, -off)
		return time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, time.Local)
	}
	thisMon := monday(now)
	start := monday(now.AddDate(0, 0, -21)) // 0주 = 3주 전 월요일 → 지금은 사이클 1의 2주 차
	cycleStart := start.AddDate(0, 0, 14)   // cycleStartWeek 2
	lastMon := thisMon.AddDate(0, 0, -7)
	lastFri := lastMon.AddDate(0, 0, 4)

	settings := doc{
		"startDate":      ymd(start),
		"cycleStartWeek": 2,
		"cycleWeeks":     6,
		"cooldownWeeks":  1,
		"people": []any{
			doc{"id": "p1", "label": "A", "leadCap": 2},
			doc{"id": "p2", "label": "B", "leadCap": 2},
			doc{"id": "p3", "label": "C", "leadCap": 1},
			doc{"id": "p4", "label": "D", "leadCap": 1},
		},
		"slotCap":        2,
		"requesterRoles": []any{"다른 파트 개발자", "과제 PM", "다른 파트", "팀장 지시", "운영 담당", "기타"},
		"onboarded":      false,
	}
	if _, err := st.PutSettings(ctx, settings); err != nil {
		return err
	}

	brief := func(problem, success, constraints, out string) doc {
		return doc{"problem": problem, "success": success, "constraints": constraints, "out": out}
	}
	programs := []struct {
		id string
		d  doc
	}{
		{"prog-1", doc{"order": 1, "name": "주문 처리 서비스", "system": "백엔드", "lead": "p1", "sub": "p2", "active": true, "subImpl": false,
			"brief": brief("월말에 주문 이력 조회가 느리다는 요청이 반복됩니다.", "월말 조회 3초 안, 운영 담당이 확인", "저장 구조는 이번 사이클에 바꾸지 않습니다.", "새 보고서 화면"), "note": ""}},
		{"prog-2", doc{"order": 2, "name": "로그 수집기", "system": "인프라", "lead": "p1", "sub": "p3", "active": true, "subImpl": false,
			"brief": brief("감사 대응으로 로그 보존 기간을 늘려야 합니다.", "", "", ""), "note": ""}},
		{"prog-3", doc{"order": 3, "name": "관리 화면", "system": "웹", "lead": "p2", "sub": "p1", "active": true, "subImpl": true,
			"brief": brief("", "", "", ""), "note": "서브가 목록 화면을 나눠 구현"}},
		{"prog-4", doc{"order": 4, "name": "데이터 변환기", "system": "데이터", "lead": "p3", "sub": "p4", "active": true, "subImpl": true,
			"brief": brief("변환 규칙을 요청마다 손으로 추가하고 있습니다.", "규칙 추가가 반나절 안에 끝남", "주니어 리드: 입력 형식 변경은 파트 리드와 상의", "화면 편집기"), "note": ""}},
		{"prog-5", doc{"order": 5, "name": "시험 자동화", "system": "품질", "lead": "p4", "sub": "p2", "active": true, "subImpl": false,
			"brief": brief("", "", "", ""), "note": ""}},
		{"prog-6", doc{"order": 6, "name": "배포 도구", "system": "인프라", "lead": "p2", "sub": "p4", "active": true, "subImpl": false,
			"brief": brief("", "", "", ""), "note": ""}},
		{"prog-7", doc{"order": 7, "name": "문서 검색", "system": "검색", "lead": "p3", "sub": "p1", "active": false, "subImpl": false,
			"brief": brief("", "", "", ""), "note": "대기: 다음 사이클 후보"}},
	}
	for _, p := range programs {
		if _, err := st.Put(ctx, "programs", p.id, p.d); err != nil {
			return err
		}
	}

	req := func(r doc) doc {
		base := doc{"raw": "", "what": "", "from": "다른 파트 개발자", "program": "", "size": "M", "why": "", "when": "", "check": "",
			"urgent": false, "status": "접수", "askedAt": nil, "forwardedAt": nil, "closedAt": nil, "heldAt": nil,
			"absorbed": false, "absorbedAt": nil, "est": nil, "note": ""}
		for k, v := range r {
			base[k] = v
		}
		if _, ok := base["createdAt"]; !ok {
			base["createdAt"] = base["receivedAt"]
		}
		return base
	}
	requests := []doc{
		req(doc{"raw": "감사 대응 때문에 로그를 90일까지 남겨 주실 수 있나요?", "what": "로그 보존 기간 90일로 늘리기", "program": "prog-2",
			"why": "감사 대응(보안팀 결정)", "check": "90일 전 로그를 조회할 수 있음", "receivedAt": iso(day(6, 10, 0))}),
		req(doc{"raw": "관리 화면에서 목록을 엑셀로 받을 수 있으면 좋겠어요.", "what": "관리 화면에 내보내기 버튼 추가", "from": "과제 PM",
			"program": "prog-3", "size": "S", "receivedAt": iso(day(0, 9, 30))}),
		req(doc{"raw": "월말에 주문 이력 화면이 너무 느립니다.", "what": "주문 이력 화면 속도 개선", "from": "운영 담당", "program": "prog-1",
			"why": "월말 조회가 느리다는 민원", "status": "정리 중", "receivedAt": iso(day(22, 14, 0)), "askedAt": iso(day(20, 10, 0))}),
		req(doc{"raw": "새 입력 형식 두 가지를 변환해 주세요.", "what": "데이터 변환 규칙 2개 추가", "from": "다른 파트", "program": "prog-4", "size": "S",
			"why": "다음 달 연동 시험", "when": "다음 달 1일, 연동 시험 시작일", "check": "예시 파일 두 개가 변환됨", "status": "리드 전달",
			"receivedAt": iso(day(3, 11, 0)), "forwardedAt": iso(day(2, 9, 30)), "est": doc{"min": 1, "mid": 2, "max": 3}}),
		req(doc{"raw": "시험 결과를 CSV로도 남겨 주세요.", "what": "시험 결과 파일 형식 바꾸기", "program": "prog-5", "size": "S",
			"why": "결과 비교 자동화", "when": "이번 달 안", "check": "CSV 파일 생성", "status": "완료",
			"receivedAt": iso(day(10, 10, 0)), "forwardedAt": iso(day(9, 15, 0)), "closedAt": iso(day(4, 17, 0))}),
		req(doc{"raw": "배포 뒤 일부 사용자가 접속이 안 됩니다.", "what": "배포 후 접속 오류 대응", "from": "운영 담당", "program": "prog-6",
			"why": "운영 중 서비스 장애", "when": "즉시", "check": "접속 정상화", "urgent": true, "status": "진행", "receivedAt": iso(day(0, 8, 40))}),
		req(doc{"raw": "회의 자료에 들어갈 숫자 좀 정리해 주세요.", "what": "회의 자료 숫자 정리", "from": "팀장 지시", "size": "S",
			"why": "주간 회의", "when": "내일 오전", "check": "표 한 장", "status": "완료", "absorbed": true, "absorbedAt": iso(day(1, 16, 0)),
			"receivedAt": iso(day(1, 13, 0)), "closedAt": iso(day(1, 16, 0))}),
	}
	for i, r := range requests {
		if _, err := st.Put(ctx, "requests", fmt.Sprintf("req-%d", i+1), r); err != nil {
			return err
		}
	}

	decisions := []doc{
		{"date": ymd(day(6, 0, 0)), "text": "로그 보존은 수집기 설정으로 해결하고 저장 구조는 바꾸지 않음", "why": "다른 프로그램에 영향이 없음", "by": "프로그램 리드", "program": "prog-2"},
		{"date": ymd(day(3, 0, 0)), "text": "관리 화면 내보내기는 다음 사이클로 미룸", "why": "이번 사이클 자리 계산상 여유 없음", "by": "파트 리드", "program": "prog-3"},
		{"date": ymd(day(1, 0, 0)), "text": "배포 도구 장애 대응 뒤 30분 회고", "why": "같은 오류 재발 방지", "by": "프로그램 리드", "program": "prog-6"},
	}
	for i, d := range decisions {
		if _, err := st.Put(ctx, "decisions", fmt.Sprintf("dec-%d", i+1), d); err != nil {
			return err
		}
	}

	mapitems := []doc{
		{"item": "보존 기간 설정 변경", "program": "prog-2", "status": "설계", "risk": "저장 공간 증가 확인 필요", "decide": "", "done": false, "updatedAt": iso(day(1, 17, 0))},
		{"item": "목록 내보내기 버튼", "program": "prog-3", "status": "대기", "risk": "", "decide": "이번 사이클에 넣을지 (파트 리드)", "done": false, "updatedAt": iso(day(2, 10, 0))},
		{"item": "변환 규칙 추가", "program": "prog-4", "status": "구현", "risk": "", "decide": "", "done": false, "updatedAt": iso(day(1, 11, 0))},
		{"item": "배포 스크립트 점검", "program": "prog-6", "status": "진행", "risk": "같은 장애 재발 가능성", "decide": "", "done": false, "updatedAt": iso(day(0, 10, 0))},
	}
	for i, d := range mapitems {
		if _, err := st.Put(ctx, "mapitems", fmt.Sprintf("map-%d", i+1), d); err != nil {
			return err
		}
	}

	timelog := []doc{
		{"date": ymd(day(2, 0, 0)), "kind": "접수·명확화·회신", "hours": 0.5},
		{"date": ymd(day(2, 0, 0)), "kind": "통합 설계", "hours": 2},
		{"date": ymd(day(1, 0, 0)), "kind": "접수·명확화·회신", "hours": 1},
		{"date": ymd(day(1, 0, 0)), "kind": "멘토링", "hours": 0.5},
		{"date": ymd(lastFri), "kind": "금요 읽기", "hours": 1.25},
	}
	for i, d := range timelog {
		if _, err := st.Put(ctx, "timelog", fmt.Sprintf("time-%d", i+1), d); err != nil {
			return err
		}
	}

	blockers := []doc{
		{"kind": "b", "text": "다른 파트 승인이 있어야 시험 장비를 쓸 수 있음", "receivedAt": iso(day(5, 14, 0)), "state": "open",
			"pausedDays": 0, "pausedAt": nil, "wasPaused": false, "resolvedAt": nil, "createdAt": iso(day(5, 14, 0)), "program": "prog-5"},
		{"kind": "c", "text": "설정 파일 위치가 두 곳이라 어느 쪽을 고쳐야 할지 헷갈림", "receivedAt": iso(day(0, 11, 0)), "state": "open",
			"pausedDays": 0, "pausedAt": nil, "wasPaused": false, "resolvedAt": nil, "createdAt": iso(day(0, 11, 0)), "program": "prog-4"},
	}
	for i, d := range blockers {
		if _, err := st.Put(ctx, "blockers", fmt.Sprintf("blk-%d", i+1), d); err != nil {
			return err
		}
	}

	notes := []doc{
		{"kind": "q", "text": "배포 도구 진행 상황을 물어볼 뻔함 — 진행 지도의 위험 칸이 비어 있었음", "createdAt": iso(day(1, 15, 0)), "done": false},
		{"kind": "l", "text": "다음 1:1에서 서브 구현 범위 다시 보기", "createdAt": iso(day(2, 18, 0)), "done": false},
	}
	for i, d := range notes {
		if _, err := st.Put(ctx, "notes", fmt.Sprintf("note-%d", i+1), d); err != nil {
			return err
		}
	}

	if _, err := st.Put(ctx, "weeks", ymd(thisMon), doc{"signals": doc{"reviewEmpty": true}, "note": "리뷰어 없이 병합된 PR 1건"}); err != nil {
		return err
	}
	if _, err := st.Put(ctx, "cycles", ymd(cycleStart), doc{"checks": doc{"0": true, "1": true, "2": true}}); err != nil {
		return err
	}
	friday := doc{
		"v":    2,
		"week": ymd(lastMon),
		"draft": "주간 보고 (" + ymd(lastFri) + ")\n\n1. 위험 (진행 지도 위험 칸)\n- 보존 기간 설정 변경 (로그 수집기): 저장 공간 증가 확인 필요\n\n" +
			"2. 결정 필요\n- 없음\n\n3. 접수 현황\n- 접수 대기 2건 · 전달 2일 초과 0건 · 이번 주 새 요청 3건 · 이번 주 완료 1건\n- 활성 프로그램 6개\n\n" +
			"4. 이번 주 흡수 건수: 0건\n\n개인별 진행률·업무량은 적지 않습니다.",
		"map":     []any{doc{"item": "보존 기간 설정 변경", "program": "prog-2", "status": "설계", "risk": "저장 공간 증가 확인 필요", "decide": ""}},
		"counts":  doc{"waiting": 2, "over": 0, "absorbed": 0, "active": 6},
		"savedAt": iso(time.Date(lastFri.Year(), lastFri.Month(), lastFri.Day(), 17, 0, 0, 0, time.Local)),
	}
	if _, err := st.Put(ctx, "fridays", ymd(lastMon), friday); err != nil {
		return err
	}
	return nil
}
