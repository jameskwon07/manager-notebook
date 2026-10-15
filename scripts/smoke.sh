#!/usr/bin/env bash
# manager-notebook 연기 시험(smoke test) — SPEC §9-3
#
# 쓰는 법
#   scripts/smoke.sh [이미지]                 # 기본 이미지: manager-notebook:dev
#   BUILD=1 scripts/smoke.sh                  # 먼저 docker build 를 하고 시험
#   KEEP=1 scripts/smoke.sh                   # 끝나도 컨테이너·볼륨을 지우지 않음(문제 볼 때)
#   BASE_URL=http://127.0.0.1:8080 SMOKE_ALLOW_WIPE=1 scripts/smoke.sh
#                                             # 이미 떠 있는 서버(비밀번호 없음)를 시험.
#                                             # import 시험이 그 서버의 데이터를 모두 바꿉니다.
#
# 무엇을 하나
#   1. 컨테이너를 --network none --read-only 로 띄웁니다(외부 연결이 없어도 도는지 확인).
#      --network none 에서는 -p 포트 공개가 동작하지 않습니다. 그래서 공개 curl 이미지를
#      같은 네트워크 공간(--network container:<앱>)에 붙여 그 안에서 요청합니다.
#   2. health, CRUD, PATCH null 삭제, export→import 왕복, ics, guide, 보안 헤더,
#      정적 파일, LLM 꺼짐 404, 크기 제한, 로그에 본문 없음, 이미지 설정을 확인합니다.
#   3. 같은 볼륨으로 NOTEBOOK_PASSWORD 를 켠 컨테이너를 다시 띄워
#      데이터 유지, 401, 로그인·로그아웃을 확인합니다.
#
# 필요한 것: bash, docker, python3 (BASE_URL 모드에서는 docker 대신 curl)
# 결과: 실패가 하나라도 있으면 종료 코드 1, 준비물이 없으면 2.

set -euo pipefail

SCRIPT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
ROOT_DIR=$(cd "$SCRIPT_DIR/.." && pwd)

IMAGE=${1:-${IMAGE:-manager-notebook:dev}}
CURL_IMAGE=${CURL_IMAGE:-curlimages/curl:8.16.0}
# 시험이 넣는 설정(쉬는 날 12-25·01-01뿐)에는 첫째 월요일 휴일이 없어 옮긴 1:1 안내가 없습니다: 반복 5 + 표지 7.
# (기본 쉬는 날 목록이면 옮긴 1:1 안내 3개가 더해져 15개입니다.)
ICS_EXPECTED_EVENTS=${ICS_EXPECTED_EVENTS:-12}
EXPECTED_VERSION=${EXPECTED_VERSION:-}
BUILD=${BUILD:-0}
KEEP=${KEEP:-0}
BASE_URL=${BASE_URL:-}
SMOKE_ALLOW_WIPE=${SMOKE_ALLOW_WIPE:-0}

RUN_ID="nb-smoke-$$-$RANDOM"
APP="$RUN_ID-app"
APP_AUTH="$RUN_ID-auth"
HELPER=""
VOL="$RUN_ID-data"
PASSWORD="smoke-$RANDOM-$RANDOM-pass"
SECRET_MARK="SMOKE-SECRET-$RANDOM$RANDOM"

PASS=0
FAIL=0
WARN=0
COOKIE=""
MODE=docker
[[ -n $BASE_URL ]] && MODE=url

T=$(mktemp -d "${TMPDIR:-/tmp}/nb-smoke.XXXXXX")

# ---------------------------------------------------------------- 출력
ok()   { PASS=$((PASS + 1)); printf '  ok    %s\n' "$1"; }
bad()  { FAIL=$((FAIL + 1)); printf '  FAIL  %s\n' "$1"; [[ -n ${2:-} ]] && printf '        %s\n' "$2"; return 0; }
note() { WARN=$((WARN + 1)); printf '  warn  %s\n' "$1"; [[ -n ${2:-} ]] && printf '        %s\n' "$2"; return 0; }
step() { printf '\n== %s\n' "$1"; }
die()  { printf 'smoke: %s\n' "$1" >&2; exit 2; }

# ---------------------------------------------------------------- 정리
cleanup() {
  local code=$?
  if [[ $MODE == docker ]]; then
    if [[ $KEEP == 1 ]]; then
      printf '\nKEEP=1: 남겨 둔 것 — 컨테이너 %s / %s, 볼륨 %s\n' "$APP" "$APP_AUTH" "$VOL"
    else
      docker rm -f "$APP-curl" "$APP_AUTH-curl" "$APP" "$APP_AUTH" >/dev/null 2>&1 || true
      docker volume rm -f "$VOL" >/dev/null 2>&1 || true
    fi
  fi
  rm -rf "$T"
  exit "$code"
}
trap cleanup EXIT
trap 'exit 130' INT TERM

# ---------------------------------------------------------------- 준비물
command -v python3 >/dev/null || die "python3 가 필요합니다."
if [[ $MODE == docker ]]; then
  command -v docker >/dev/null || die "docker 가 필요합니다."
  docker info >/dev/null 2>&1 || die "Docker 엔진에 연결할 수 없습니다. Docker Desktop(또는 Rancher Desktop)을 켜 주세요."
else
  command -v curl >/dev/null || die "curl 이 필요합니다."
  [[ $SMOKE_ALLOW_WIPE == 1 ]] || die "BASE_URL 모드는 그 서버의 데이터를 모두 바꿉니다. 괜찮으면 SMOKE_ALLOW_WIPE=1 을 함께 주세요."
fi

# ---------------------------------------------------------------- 파이썬 도우미
cat >"$T/h.py" <<'PY'
import json, os, re, sys

T = os.environ["SMOKE_T"]

def read(name, mode="r"):
    try:
        with open(os.path.join(T, name), mode, encoding=None if "b" in mode else "utf-8", errors=None if "b" in mode else "replace") as f:
            return f.read()
    except FileNotFoundError:
        return b"" if "b" in mode else ""

def split():
    raw = read("raw", "rb")
    status, headers, body = 0, [], raw
    # curl -i 출력: (HTTP/1.1 100 Continue 블록이 앞에 올 수 있음) 마지막 헤더 블록 뒤가 본문
    while body.startswith(b"HTTP/"):
        sep = body.find(b"\r\n\r\n")
        if sep < 0:
            head, body = body, b""
        else:
            head, body = body[:sep], body[sep + 4:]
        lines = head.decode("latin-1").split("\r\n")
        try:
            status = int(lines[0].split()[1])
        except Exception:
            status = 0
        headers = [l for l in lines[1:] if ":" in l]
        if status not in (100, 101):
            break
    with open(os.path.join(T, "status"), "w") as f:
        f.write(str(status))
    with open(os.path.join(T, "headers"), "w", encoding="latin-1") as f:
        f.write("\n".join(headers))
    with open(os.path.join(T, "body"), "wb") as f:
        f.write(body)

def ctx():
    hs = []
    for line in read("headers").split("\n"):
        if ":" in line:
            k, v = line.split(":", 1)
            hs.append((k.strip().lower(), v.strip()))
    h = {}
    for k, v in hs:
        h[k] = v
    t = read("body", "rb").decode("utf-8", "replace")
    try:
        d = json.loads(t)
    except Exception:
        d = None
    s = int(read("status") or 0)
    def hall(name):
        return [v for k, v in hs if k == name.lower()]
    def load(name):
        with open(os.path.join(T, name), encoding="utf-8") as f:
            return json.load(f)
    return {"h": h, "hs": hs, "hall": hall, "t": t, "d": d, "s": s, "re": re,
            "json": json, "load": load, "env": os.environ}

def unfold(t):
    return t.replace("\r\n ", "").replace("\r\n\t", "").replace("\n ", "").replace("\n\t", "")

cmd = sys.argv[1]
if cmd == "split":
    split()
elif cmd in ("eval", "val"):
    c = ctx()
    c["unfold"] = unfold
    try:
        r = eval(sys.argv[2], c)
    except Exception as e:  # 식 자체가 깨지면 실패로 봄
        print(f"{type(e).__name__}: {e}")
        sys.exit(2)
    if cmd == "val":
        print(r if isinstance(r, str) else json.dumps(r, ensure_ascii=False))
        sys.exit(0)
    sys.exit(0 if r else 1)
elif cmd == "now":
    import time
    print(time.time())
elif cmd == "big":
    # big <파일> <바이트 수> : 큰 JSON 본문 만들기
    n = int(sys.argv[3])
    with open(sys.argv[2], "w") as f:
        json.dump({"kind": "q", "text": "x" * n, "createdAt": "2026-10-19T09:00:00+09:00", "done": False}, f)
elif cmd == "compare_export":
    # 가져온 뒤 state 가 내보낸 파일과 같은지
    exp = json.load(open(os.path.join(T, "export.json"), encoding="utf-8"))
    st = json.loads(read("body", "rb").decode("utf-8"))
    def norm(docs):
        return sorted(json.dumps(x, sort_keys=True, ensure_ascii=False) for x in (docs or []))
    bad = [k for k in ("notes", "blockers", "fridays") if norm(exp.get(k)) != norm(st.get(k))]
    if json.dumps(exp.get("settings"), sort_keys=True) != json.dumps(st.get("settings"), sort_keys=True):
        bad.append("settings")
    if bad:
        print("다른 곳: " + ", ".join(bad))
        sys.exit(1)
PY
export SMOKE_T="$T"
py() { python3 "$T/h.py" "$@"; }

# ---------------------------------------------------------------- HTTP
# req METHOD PATH [본문파일] [추가 헤더...]
# 결과는 $T/status, $T/headers, $T/body 에 남습니다.
req() {
  local method=$1 path=$2 body=${3:-}
  shift 2; [[ $# -gt 0 ]] && shift
  local args=(-s -i -X "$method" -H 'Expect:' --max-time 30)
  [[ -n $COOKIE ]] && args+=(-H "Cookie: $COOKIE")
  local h
  for h in "$@"; do args+=(-H "$h"); done
  local input=/dev/null
  if [[ -n $body ]]; then
    args+=(-H 'Content-Type: application/json' --data-binary @-)
    input=$body
  fi
  : >"$T/raw"
  if [[ $MODE == docker ]]; then
    docker exec -i "$HELPER" curl "${args[@]}" "http://127.0.0.1:8080$path" <"$input" >"$T/raw" 2>/dev/null || true
  else
    curl "${args[@]}" "${BASE_URL%/}$path" <"$input" >"$T/raw" 2>/dev/null || true
  fi
  py split
}

status() { cat "$T/status"; }
val() { py val "$1"; }

# expect_status <코드|코드,코드> <설명>
expect_status() {
  local want=$1 desc=$2 got
  got=$(status)
  if [[ ",$want," == *",$got,"* ]]; then ok "$desc ($got)"; else bad "$desc: 상태 $got (기대 $want)" "$(head -c 300 "$T/body" | tr '\n' ' ')"; fi
}

# expect <파이썬 식> <설명>   — 식에서 쓸 수 있는 이름: d(JSON), t(본문 글), h(헤더, 소문자 키), s(상태), hall(), re, unfold()
expect() {
  local expr=$1 desc=$2 out
  if out=$(py eval "$expr" 2>&1); then ok "$desc"; else bad "$desc" "$out"; fi
}
soft() {
  local expr=$1 desc=$2 out
  if out=$(py eval "$expr" 2>&1); then ok "$desc"; else note "$desc" "$out"; fi
}

json_file() { printf '%s' "$2" >"$T/$1"; printf '%s' "$T/$1"; }

# ---------------------------------------------------------------- 컨테이너
# start_app <이름> [docker run 추가 인자...]
start_app() {
  local name=$1; shift
  docker run -d --name "$name" \
    --network none \
    --read-only \
    --cap-drop ALL \
    --security-opt no-new-privileges:true \
    -v "$VOL:/data" \
    "$@" \
    "$IMAGE" >/dev/null
  HELPER="$name-curl"
  docker run -d --name "$HELPER" --network "container:$name" \
    --entrypoint sleep "$CURL_IMAGE" 1800 >/dev/null
}

wait_health() {
  local i
  for i in $(seq 1 40); do
    if [[ $MODE == docker ]] && [[ $(docker inspect -f '{{.State.Running}}' "$1" 2>/dev/null) != true ]]; then
      bad "컨테이너가 멈췄습니다" "$(docker logs --tail 30 "$1" 2>&1 | tr '\n' ' ')"
      return 1
    fi
    req GET /api/health
    [[ $(status) == 200 ]] && return 0
    sleep 0.5
  done
  bad "health 가 20초 안에 200 이 되지 않았습니다" "$(head -c 300 "$T/body")"
  return 1
}

# ================================================================ 시작
printf 'manager-notebook smoke — 모드: %s\n' "$MODE"

if [[ $MODE == docker ]]; then
  if [[ $BUILD == 1 ]]; then
    step "이미지 빌드: $IMAGE"
    docker build -t "$IMAGE" "$ROOT_DIR"
  fi
  docker image inspect "$IMAGE" >/dev/null 2>&1 || die "이미지 $IMAGE 가 없습니다. BUILD=1 로 다시 실행하거나 먼저 docker build 하세요."
  docker image inspect "$CURL_IMAGE" >/dev/null 2>&1 || docker pull -q "$CURL_IMAGE" >/dev/null || die "$CURL_IMAGE 를 받을 수 없습니다."

  step "이미지 설정"
  docker image inspect "$IMAGE" --format '{{json .Config}}' >"$T/body"
  printf '200' >"$T/status"; : >"$T/headers"
  expect "d['User'] in ('nonroot','nonroot:nonroot','65532','65532:65532')" "비루트 사용자(User=$(val "d.get('User')"))"
  expect "'8080/tcp' in (d.get('ExposedPorts') or {})" "EXPOSE 8080"
  expect "'/data' in (d.get('Volumes') or {})" "VOLUME /data"
  expect "(d.get('Healthcheck') or {}).get('Test',[])[-2:] == ['/manager-notebook','healthcheck'] or (d.get('Healthcheck') or {}).get('Test',[])[-1:] == ['healthcheck']" "HEALTHCHECK = /manager-notebook healthcheck"
  expect "'TZ=Asia/Seoul' in d.get('Env',[]) and 'NOTEBOOK_DATA_DIR=/data' in d.get('Env',[])" "ENV TZ, NOTEBOOK_DATA_DIR"
  expect "(d.get('Labels') or {}).get('org.opencontainers.image.licenses') == 'MIT'" "OCI 라벨 licenses=MIT"
  expect "all((d.get('Labels') or {}).get('org.opencontainers.image.'+k) for k in ('title','description','source','version'))" "OCI 라벨 title·description·source·version"
  expect "not re.search(r'(?i)(password|token|api_key|secret)=[^\\s\"]+', json.dumps(d.get('Env',[])))" "이미지 ENV 에 비밀값 없음"

  step "컨테이너 시작 (--network none --read-only --cap-drop ALL)"
  docker volume create "$VOL" >/dev/null
  start_app "$APP"
  wait_health "$APP" || exit 1
  ok "컨테이너가 떠서 health 응답"

  if out=$(docker exec "$APP" /manager-notebook healthcheck 2>&1); then ok "healthcheck 하위 명령 종료 코드 0"; else bad "healthcheck 하위 명령 실패" "$out"; fi
  if out=$(docker exec "$APP" /manager-notebook version 2>&1); then ok "version 하위 명령: $(printf '%s' "$out" | head -1)"; else bad "version 하위 명령 실패" "$out"; fi
else
  step "서버 확인: $BASE_URL"
  wait_health "" || exit 1
fi

# ---------------------------------------------------------------- health
step "health"
req GET /api/health
expect_status 200 "GET /api/health"
expect "d.get('ok') is True and isinstance(d.get('version'), str) and d['version']" "본문 {ok:true, version}"
if [[ -n $EXPECTED_VERSION ]]; then
  expect "d.get('version') == env['EXPECTED_VERSION']" "버전 = $EXPECTED_VERSION"
fi

# ---------------------------------------------------------------- 정적 파일
step "정적 파일"
for spec in "/|text/html" "/app.js|javascript" "/app.css|text/css" "/content.json|json" "/favicon.svg|image/svg+xml" "/login|text/html"; do
  path=${spec%%|*}; ctype=${spec##*|}
  req GET "$path"
  if [[ $path == /login && $(status) =~ ^30[1237]$ ]]; then
    # 비밀번호가 없으면 /login 이 / 로 돌려보낼 수 있습니다. 인증 단계에서 200 을 다시 봅니다.
    ok "GET /login → $(status) (비밀번호 없음이라 돌려보냄)"
    continue
  fi
  expect_status 200 "GET $path"
  expect "'$ctype' in h.get('content-type','')" "  Content-Type 에 '$ctype' ($(val "h.get('content-type','')"))"
  case $path in
    /content.json) expect "isinstance(d, dict) and all(k in d for k in ('capture','blocker','friday','prompts'))" "  content.json 이 JSON 이고 기본 키가 있음" ;;
  esac
  if [[ $path == / || $path == /login || $path == /app.css || $path == /app.js ]]; then
    expect "not re.search(r'(?i)(<script[^>]+src=[\"\\']?https?:|<link[^>]+href=[\"\\']?https?:|@import\\s+url\\(\\s*[\"\\']?https?:|url\\(\\s*[\"\\']?https?:)', t)" "  외부에서 불러오는 스크립트·스타일·글꼴 없음"
    expect "not re.search(r'(?i)fonts\\.googleapis|fonts\\.gstatic|googletagmanager|google-analytics|cdnjs|jsdelivr|unpkg', t)" "  CDN·글꼴·분석 주소 없음"
    if [[ $path == / || $path == /login ]]; then
      expect "not re.search(r'(?is)<script(?![^>]*\\bsrc=)[^>]*>\\s*\\S', t)" "  인라인 스크립트 없음(CSP)"
      expect "not re.search(r'(?i)\\sstyle\\s*=', t)" "  인라인 style 속성 없음(CSP)"
    fi
  fi
  if [[ $path == /content.json || $path == / || $path == /app.js ]]; then
    soft "'claude.ai' not in t" "  'claude.ai' 문구 없음"
  fi
done

# ---------------------------------------------------------------- 보안 헤더
step "보안 헤더"
CSP="default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data:; connect-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'"
export SMOKE_CSP="$CSP"
for path in / /app.js /api/health /api/state /api/guide /api/nope; do
  req GET "$path"
  expect "h.get('content-security-policy','').strip().rstrip(';') == env['SMOKE_CSP']" "$path: CSP 가 SPEC 과 같음"
  expect "h.get('x-content-type-options','').lower() == 'nosniff'" "$path: X-Content-Type-Options nosniff"
  expect "h.get('referrer-policy','').lower() == 'no-referrer'" "$path: Referrer-Policy no-referrer"
  expect "h.get('x-frame-options','').upper() == 'DENY'" "$path: X-Frame-Options DENY"
  if [[ $path == /api/* ]]; then
    expect "'no-store' in h.get('cache-control','')" "$path: Cache-Control no-store"
  fi
done

# ---------------------------------------------------------------- 상태와 LLM 꺼짐
step "처음 상태와 LLM 꺼짐"
req GET /api/state
expect_status 200 "GET /api/state"
expect "isinstance(d, dict) and all(k in d for k in ('version','settings','notes','blockers','fridays','llm'))" "state 키 모양"
expect "d['llm'].get('enabled') is False" "llm.enabled = false (LLM_BASE_URL 없음)"

req POST /api/llm "$(json_file llm.json '{"messages":[{"role":"user","content":"안녕"}]}')"
expect_status 404 "POST /api/llm (꺼짐)"
expect "d.get('error') == 'llm_disabled' and d.get('message')" "오류 본문 {error:llm_disabled, message}"

req POST /api/login "$(json_file login0.json '{"password":"x"}')"
expect_status 404 "비밀번호 없을 때 POST /api/login 은 404"

# ---------------------------------------------------------------- CRUD
step "CRUD"
# 변수가 든 JSON 은 $( ) 밖에서 만듭니다(macOS bash 3.2 는 $( ) 안의 "{...,...}" 를 중괄호 확장함).
json_file n1.json "$(printf '{"kind":"q","text":"%s 물음 하나","createdAt":"2026-10-19T09:10:00+09:00","done":false}' "$SECRET_MARK")" >/dev/null
req POST /api/docs/notes "$T/n1.json"
expect_status 201 "POST /api/docs/notes"
expect "isinstance(d, dict) and isinstance(d.get('id'), str) and d['id'] and d.get('kind') == 'q'" "저장된 doc 에 id 가 있음"
N1=$(val "d.get('id','')")

req POST /api/docs/notes "$(json_file n2.json '{"kind":"l","text":"둘째","createdAt":"2026-10-19T09:11:00+09:00","done":false}')"
N2=$(val "d.get('id','')")
export SMOKE_N1="$N1" SMOKE_N2="$N2"
soft "env['SMOKE_N1'] < env['SMOKE_N2']" "id 가 시간순으로 정렬됨($N1 < $N2)"

req PUT /api/docs/notes/smoke-patch.1 "$(json_file n3.json '{"kind":"q","text":"고칠 메모","createdAt":"2026-10-19T09:12:00+09:00","done":false,"extra":"지울 값"}')"
expect_status 200 "PUT /api/docs/notes/smoke-patch.1 (없으면 생성)"

req PATCH /api/docs/notes/smoke-patch.1 "$(json_file p1.json '{"done":true,"extra":null}')"
expect_status 200 "PATCH /api/docs/notes/smoke-patch.1"
expect "d.get('done') is True and 'extra' not in d and d.get('text') == '고칠 메모'" "PATCH: 병합되고 null 키는 지워짐"

req PATCH /api/docs/notes/does-not-exist "$(json_file p2.json '{"done":true}')"
expect_status 404 "없는 doc PATCH 는 404"
expect "d.get('error') and d.get('message')" "오류 본문 모양 {error, message}"

req PUT /api/docs/other/x "$(json_file o1.json '{"a":1}')"
expect_status 404 "허용하지 않는 컬렉션은 404"

req PUT "/api/docs/notes/bad%20id%21" "$(json_file o2.json '{"a":1}')"
expect "400 <= s < 500" "id 형식이 틀리면 4xx ($(status))"

req PUT /api/docs/notes/smoke-put "$(json_file n4.json '{"kind":"q","text":"교체 전","createdAt":"2026-10-19T09:13:00+09:00","done":false,"old":1}')"
req PUT /api/docs/notes/smoke-put "$(json_file n5.json '{"kind":"q","text":"교체 후","createdAt":"2026-10-19T09:13:00+09:00","done":false}')"
expect_status 200 "PUT 전체 교체"
expect "d.get('text') == '교체 후' and 'old' not in d" "PUT 은 병합이 아니라 교체"

req DELETE "/api/docs/notes/$N2"
expect_status 204 "DELETE /api/docs/notes/<id>"

req POST /api/docs/blockers "$(json_file b1.json '{"kind":"b","text":"막힘 하나","receivedAt":"2026-10-19T17:00:00+09:00","state":"open","pausedDays":0,"pausedAt":null,"wasPaused":false,"resolvedAt":null,"createdAt":"2026-10-19T17:01:00+09:00"}')"
expect_status 201 "POST /api/docs/blockers"

req PUT /api/docs/fridays/2026-10-19 "$(json_file f1.json '{"week":"2026-10-19","fields":{"done":["끝난 일"],"risks":[],"decisions":[],"scope":[],"remaining":[],"gap":""},"team":[{"line":"끝난 일","source":"done"}],"boss5":[],"company":"","kept_line":"","savedAt":"2026-10-23T15:00:00+09:00"}')"
expect_status 200 "PUT /api/docs/fridays/2026-10-19"

req PUT /api/settings "$(json_file s1.json '{"startDate":"2026-10-19","cycleStartWeek":2,"cycleWeeks":6,"cooldownWeeks":1,"holidays":"2026-12-25 성탄절\n2027-01-01 새해","companyTemplate":"실적\n계획\n이슈"}')"
expect_status 200 "PUT /api/settings"

req GET /api/state
expect "sorted(x['id'] for x in d['notes']) == sorted([env['SMOKE_N1'], 'smoke-patch.1', 'smoke-put'])" "state.notes 가 만든 것과 같음(지운 것은 없음)"
expect "len(d['blockers']) == 1 and len(d['fridays']) == 1 and d['fridays'][0].get('id') == '2026-10-19'" "state.blockers·fridays"
expect "d['settings'] and d['settings'].get('startDate') == '2026-10-19' and d['settings'].get('cycleWeeks') == 6" "state.settings"

# ---------------------------------------------------------------- 크기 제한
step "크기 제한"
py big "$T/big300k.json" 300000
req PUT /api/docs/notes/smoke-big "$T/big300k.json"
expect "400 <= s < 500" "doc 하나 256KB 넘으면 4xx ($(status))"
py big "$T/big1m.json" 1100000
req POST /api/docs/notes "$T/big1m.json"
expect "400 <= s < 500" "본문 1MB 넘으면 4xx ($(status))"
req GET /api/state
expect "not any(x.get('id') == 'smoke-big' for x in d['notes']) and len(d['notes']) == 3" "거절된 큰 doc 은 저장되지 않음"

# ---------------------------------------------------------------- export → import
step "내보내기 → 가져오기 왕복"
req GET /api/export
expect_status 200 "GET /api/export"
expect "re.fullmatch(r'attachment;\\s*filename=\"manager-notebook-\\d{8}\\.json\"', h.get('content-disposition',''))" "Content-Disposition: attachment; filename=\"manager-notebook-YYYYMMDD.json\""
expect "d.get('format') == 'manager-notebook-export' and all(k in d for k in ('version','exportedAt','settings','notes','blockers','fridays'))" "export 본문 모양"
expect "len(d['notes']) == 3 and len(d['blockers']) == 1 and len(d['fridays']) == 1" "export 개수 3·1·1"
cp "$T/body" "$T/export.json"

req POST /api/docs/notes "$(json_file n6.json '{"kind":"q","text":"가져오기로 사라질 메모","createdAt":"2026-10-19T10:00:00+09:00","done":false}')"
req PUT /api/settings "$(json_file s2.json '{"startDate":"2027-01-04","cycleStartWeek":2,"cycleWeeks":6,"cooldownWeeks":1,"holidays":"","companyTemplate":""}')"

req POST /api/import "$T/export.json"
expect_status 200 "POST /api/import"
expect "d == {'notes':3,'blockers':1,'fridays':1}" "import 응답 {notes:3, blockers:1, fridays:1}"
req GET /api/state
if out=$(py compare_export 2>&1); then ok "가져온 뒤 state 가 내보낸 파일과 같음(모두 교체)"; else bad "가져온 뒤 state 가 내보낸 파일과 다름" "$out"; fi

req POST /api/import "$(json_file bad_import.json '{"format":"something-else","notes":"not-a-list"}')"
expect "400 <= s < 500" "틀린 import 파일은 4xx ($(status))"
req GET /api/state
if out=$(py compare_export 2>&1); then ok "틀린 import 뒤에도 데이터가 그대로(트랜잭션)"; else bad "틀린 import 가 데이터를 바꿈" "$out"; fi

# ---------------------------------------------------------------- ics
step "일정 파일(.ics)"
req GET /api/calendar.ics
expect_status 200 "GET /api/calendar.ics"
expect "h.get('content-type','').lower().startswith('text/calendar') and 'utf-8' in h.get('content-type','').lower()" "Content-Type text/calendar; charset=utf-8"
expect "'attachment' in h.get('content-disposition','') and 'manager-notebook.ics' in h.get('content-disposition','')" "첨부 파일 이름 manager-notebook.ics"
expect "t.startswith('BEGIN:VCALENDAR') and t.rstrip().endswith('END:VCALENDAR')" "VCALENDAR 로 감쌈"
expect "'BEGIN:VTIMEZONE' in t and 'TZID:Asia/Seoul' in unfold(t)" "VTIMEZONE Asia/Seoul"
export ICS_EXPECTED_EVENTS
expect "unfold(t).count('BEGIN:VEVENT') == int(env['ICS_EXPECTED_EVENTS'])" "VEVENT ${ICS_EXPECTED_EVENTS}개 (실제 $(val "unfold(t).count('BEGIN:VEVENT')"))"
expect "re.search(r'(?m)^EXDATE[;:]', unfold(t))" "EXDATE 가 있음"
expect "'RDATE' not in unfold(t)" "RDATE 없음(Outlook 호환)"
expect "not re.search(r'(?m)^RRULE:FREQ=MONTHLY;BYDAY=[^;]*,', unfold(t))" "서수 여러 개짜리 MONTHLY 규칙 없음(Outlook 호환)"
expect "[l for l in unfold(t).splitlines() if l.startswith('SUMMARY')] and all(l.split(':',1)[1].startswith('[관리] ') for l in unfold(t).splitlines() if l.startswith('SUMMARY'))" "모든 제목이 '[관리] '로 시작"
expect "(lambda u: len(u) == len(set(u)) and len(u) > 0 and all(x.endswith('@manager-notebook') and '2026-10-19' in x for x in u))([l.split(':',1)[1].strip() for l in unfold(t).splitlines() if l.startswith('UID')])" "UID 가 결정적이고 겹치지 않음(<key>-2026-10-19@manager-notebook)"
expect "'claude.ai' not in t" "ics 에 클라우드 링크 없음"
soft "'\\r\\n' in t" "줄 끝이 CRLF(RFC 5545)"
soft "'localhost:8080' in unfold(t)" "설명에 관리 수첩 링크(localhost:8080)가 있음"

# ---------------------------------------------------------------- guide
step "가이드"
req GET /api/guide
expect_status 200 "GET /api/guide"
expect "isinstance(d, list) and len(d) > 0 and all(isinstance(x, dict) and x.get('slug') and x.get('title') and 'order' in x for x in d)" "목록 [{slug,title,order}] (개수 $(val "len(d) if isinstance(d, list) else 0"))"
GUIDE_SLUG=$(val "d[0]['slug'] if isinstance(d, list) and d else ''")
if [[ -n $GUIDE_SLUG ]]; then
  req GET "/api/guide/$GUIDE_SLUG"
  expect_status 200 "GET /api/guide/$GUIDE_SLUG"
  expect "d.get('slug') and d.get('title') and isinstance(d.get('html'), str) and len(d['html']) > 20" "본문 {slug,title,html}"
  expect "'<script' not in d.get('html','').lower()" "본문 HTML 에 script 없음(raw HTML 꺼짐)"
fi
req GET /api/guide/no-such-guide-page
expect_status 404 "없는 가이드는 404"

# ---------------------------------------------------------------- 로그
if [[ $MODE == docker ]]; then
  step "로그"
  docker logs "$APP" >"$T/applog" 2>&1 || true
  if grep -q "$SECRET_MARK" "$T/applog"; then bad "로그에 메모 내용이 남음" "$(grep -m1 "$SECRET_MARK" "$T/applog")"; else ok "로그에 메모 내용 없음"; fi
  if grep -q 'GET /api/health 200' "$T/applog"; then bad "정상 health 요청이 로그에 남음(HEALTHCHECK 30초마다 쌓임)"; else ok "정상 health 요청은 로그에 남기지 않음"; fi
  if grep -qiE 'https?://[^ ]*(googleapis|gstatic|github|docker|update)' "$T/applog"; then note "로그에 외부 주소가 보임" "$(grep -m1 -iE 'https?://' "$T/applog")"; else ok "로그에 외부 연결 흔적 없음"; fi
fi

# ---------------------------------------------------------------- 인증 + 데이터 유지
if [[ $MODE == docker ]]; then
  step "비밀번호 켜고 같은 볼륨으로 다시 시작(데이터 유지·인증)"
  docker rm -f "$APP-curl" "$APP" >/dev/null
  start_app "$APP_AUTH" -e "NOTEBOOK_PASSWORD=$PASSWORD"
  wait_health "$APP_AUTH" || exit 1
  COOKIE=""

  req GET /api/health
  expect_status 200 "쿠키 없이 /api/health"
  for path in / /login /app.js /app.css /content.json /favicon.svg; do
    req GET "$path"
    expect_status 200 "쿠키 없이 정적 파일 $path"
  done
  req GET /login
  expect "'text/html' in h.get('content-type','') and not re.search(r'(?is)<script(?![^>]*\\bsrc=)[^>]*>\\s*\\S', t) and not re.search(r'(?i)\\sstyle\\s*=', t)" "/login 은 HTML 이고 인라인 스크립트·style 없음"
  for spec in "GET /api/state" "GET /api/export" "GET /api/calendar.ics" "GET /api/guide" "PUT /api/settings" "POST /api/docs/notes" "POST /api/llm" "POST /api/import"; do
    m=${spec%% *}; p=${spec#* }
    if [[ $m == GET ]]; then req "$m" "$p"; else req "$m" "$p" "$(json_file empty.json '{}')"; fi
    expect_status 401 "쿠키 없이 $spec"
  done
  req GET /api/state
  expect "d.get('error') == 'auth' and d.get('message')" "401 본문 {error:auth, message}"

  t0=$(py now)
  req POST /api/login "$(json_file wrong.json '{"password":"wrong-password"}')"
  t1=$(py now)
  expect "s != 200 and 400 <= s < 500" "틀린 비밀번호는 4xx ($(status))"
  expect "not any('set-cookie' == k for k, _ in hs)" "틀린 비밀번호에는 쿠키를 주지 않음"
  export SMOKE_DT
  SMOKE_DT=$(python3 -c "print($t1 - $t0)")
  soft "float(env['SMOKE_DT']) >= 0.9" "실패하면 1초 지연 ($(printf '%.2f' "$SMOKE_DT")초)"

  json_file right.json "$(printf '{"password":"%s"}' "$PASSWORD")" >/dev/null
  req POST /api/login "$T/right.json"
  expect_status 200 "맞는 비밀번호로 로그인"
  expect "hall('set-cookie') and 'httponly' in hall('set-cookie')[0].lower() and 'samesite=strict' in hall('set-cookie')[0].lower()" "쿠키가 HttpOnly, SameSite=Strict"
  soft "re.search(r'(?i)max-age=\\d+|expires=', hall('set-cookie')[0] if hall('set-cookie') else '')" "쿠키에 수명(Max-Age 또는 Expires)이 있음"
  COOKIE=$(val "hall('set-cookie')[0].split(';',1)[0].strip() if hall('set-cookie') else ''")
  [[ -n $COOKIE ]] || bad "로그인 응답에서 쿠키를 읽지 못함"

  req GET /api/state
  expect_status 200 "쿠키로 GET /api/state"
  expect "len(d['notes']) == 3 and len(d['blockers']) == 1 and len(d['fridays']) == 1 and d['settings'].get('startDate') == '2026-10-19'" "컨테이너를 지우고 다시 만들어도 데이터가 남음(볼륨)"
  req GET /api/calendar.ics
  expect_status 200 "쿠키로 GET /api/calendar.ics"

  req POST /api/logout
  expect "200 <= s < 300" "POST /api/logout ($(status))"
  soft "hall('set-cookie') and re.search(r'(?i)max-age=(0|-1)\\b|expires=thu, 01 jan 1970', hall('set-cookie')[0])" "로그아웃 응답이 쿠키를 지움"
  req GET /api/state
  soft "s == 401" "로그아웃한 쿠키로는 401 (서버 쪽 세션도 끝남)"
  COOKIE=""
fi

# ================================================================ 결과
printf '\n결과: 통과 %d, 실패 %d, 주의 %d\n' "$PASS" "$FAIL" "$WARN"
if [[ $FAIL -gt 0 ]]; then
  if [[ $MODE == docker ]]; then
    printf '\n마지막 컨테이너 로그(끝 20줄):\n'
    docker logs --tail 20 "$APP_AUTH" 2>&1 || docker logs --tail 20 "$APP" 2>&1 || true
  fi
  exit 1
fi
exit 0
