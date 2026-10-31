#!/usr/bin/env bash
# manager-notebook 2.0 연기 시험(smoke test) — SPEC §9-3, SPEC2 §3·§6·§10
#
# 쓰는 법
#   scripts/smoke.sh [이미지]                 # 기본 이미지: manager-notebook:dev
#   BUILD=1 scripts/smoke.sh                  # 먼저 docker build 를 하고 시험
#   EXPECTED_VERSION=2.1.0 scripts/smoke.sh   # health·export 의 버전까지 확인
#   KEEP=1 scripts/smoke.sh                   # 끝나도 컨테이너·볼륨을 지우지 않음(문제 볼 때)
#   BASE_URL=http://127.0.0.1:18080 SMOKE_ALLOW_WIPE=1 scripts/smoke.sh
#                                             # 이미 떠 있는 시험용 서버(비밀번호 없음)를 시험.
#                                             # import 시험이 그 서버의 데이터를 모두 바꿉니다.
#                                             # 매일 쓰는 수첩(8080)에는 쓰지 마세요.
#   ICS_EXPECTED_EVENTS=<n> scripts/smoke.sh  # VEVENT 개수(기본 30: 시험이 저장한 설정 2026-10-19·2/6/1 기준.
#                                             #  0 이면 개수는 보지 않음)
#
# 무엇을 하나
#   1. 컨테이너를 --network none --read-only 로 띄웁니다(외부 연결이 없어도 도는지 확인).
#      --network none 에서는 -p 포트 공개가 동작하지 않습니다. 그래서 공개 curl 이미지를
#      같은 네트워크 공간(--network container:<앱>)에 붙여 그 안에서 요청합니다.
#      호스트 포트를 쓰지 않으므로 이미 떠 있는 수첩과 부딪히지 않습니다.
#   2. health·버전, /api/state 11개 컬렉션 키, 1.0 컬렉션 CRUD, 2.0 컬렉션 8개 각각의
#      POST·PATCH(null 은 키 삭제)·DELETE, 없는 컬렉션 404, 정적 파일(/core.js 포함),
#      export 11키 → import 왕복, 1.0 모양 파일 import(그 셋만 교체), 일부 컬렉션만 든 import,
#      틀린 import, 일정 파일(.ics 2.0), 안내 4개, 보안 헤더, LLM 꺼짐 404, 크기 제한,
#      로그에 본문 없음, 이미지 설정을 확인합니다.
#   3. 같은 볼륨으로 NOTEBOOK_PASSWORD 를 켠 컨테이너를 다시 띄워
#      데이터 유지(11개 컬렉션·설정), 401, 로그인·로그아웃을 확인합니다.
#
# 만드는 것: 컨테이너 nb2-smoke-<번호>-app / -auth / -*-curl, 볼륨 nb2-smoke-<번호>-data.
#           끝나면 모두 지웁니다(KEEP=1 이면 남김).
# 필요한 것: bash(3.2 이상), docker, python3 (BASE_URL 모드에서는 docker 대신 curl)
# 결과: 실패가 하나라도 있으면 종료 코드 1, 준비물이 없으면 2.

set -euo pipefail

SCRIPT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
ROOT_DIR=$(cd "$SCRIPT_DIR/.." && pwd)

IMAGE=${1:-${IMAGE:-manager-notebook:dev}}
CURL_IMAGE=${CURL_IMAGE:-curlimages/curl:8.16.0}
ICS_EXPECTED_EVENTS=${ICS_EXPECTED_EVENTS:-30}
EXPECTED_VERSION=${EXPECTED_VERSION:-}
BUILD=${BUILD:-0}
KEEP=${KEEP:-0}
BASE_URL=${BASE_URL:-}
SMOKE_ALLOW_WIPE=${SMOKE_ALLOW_WIPE:-0}

RUN_ID="nb2-smoke-$$-$RANDOM"
APP="$RUN_ID-app"
APP_AUTH="$RUN_ID-auth"
HELPER=""
VOL="$RUN_ID-data"
PASSWORD="smoke-$RANDOM-$RANDOM-pass"
SECRET_MARK="SMOKE-SECRET-$RANDOM$RANDOM"

# 1.0 컬렉션 3개 + 2.0 컬렉션 8개 (SPEC2 §3)
OLD_COLLS="notes blockers fridays"
NEW_COLLS="requests programs decisions mapitems timelog weeks cycles risks"
ALL_COLLS="$OLD_COLLS $NEW_COLLS"
csv() { printf '%s' "$*" | tr ' ' ','; }
ALL_CSV=$(csv $ALL_COLLS)
OLD_CSV=$(csv $OLD_COLLS)
NEW_CSV=$(csv $NEW_COLLS)
export SMOKE_ALL="$ALL_COLLS" SMOKE_NEW="$NEW_COLLS"

PASS=0
FAIL=0
WARN=0
COOKIE=""
MODE=docker
[[ -n $BASE_URL ]] && MODE=url

T=$(mktemp -d "${TMPDIR:-/tmp}/nb2-smoke.XXXXXX")

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
      docker rm -f "$APP-curl" "$APP_AUTH-curl" "$APP" "$APP_AUTH" "$APP-demo" "$APP-demo-curl" >/dev/null 2>&1 || true
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

def load(name):
    p = name if os.path.isabs(name) else os.path.join(T, name)
    with open(p, encoding="utf-8") as f:
        return json.load(f)

def unfold(t):
    return t.replace("\r\n ", "").replace("\r\n\t", "").replace("\n ", "").replace("\n\t", "")

def events(t):
    # VEVENT 마다 펼친 줄 목록(VALARM 안의 줄도 포함)
    out, cur = [], None
    for l in unfold(t).replace("\r\n", "\n").split("\n"):
        if l == "BEGIN:VEVENT":
            cur = []
        elif l == "END:VEVENT":
            if cur is not None:
                out.append(cur)
            cur = None
        elif cur is not None:
            cur.append(l)
    return out

def prop(lines, name):
    # 'NAME:값' 또는 'NAME;파라미터:값' 줄들
    return [l for l in lines if l.split(":", 1)[0].split(";", 1)[0] == name]

def script_srcs(t):
    # [(src, defer)] — <script src=...> 순서대로
    out = []
    for tag in re.findall(r"(?is)<script\b[^>]*>", t):
        m = re.search(r"""\bsrc\s*=\s*["']?([^"'\s>]+)""", tag)
        if m:
            out.append((m.group(1), bool(re.search(r"\bdefer\b", tag))))
    return out

def norm(docs):
    return sorted(json.dumps(x, sort_keys=True, ensure_ascii=False) for x in (docs or []))

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
    return {"h": h, "hs": hs, "hall": hall, "t": t, "d": d, "s": s, "re": re,
            "json": json, "load": load, "env": os.environ, "unfold": unfold,
            "events": events, "prop": prop, "script_srcs": script_srcs, "norm": norm,
            "ALL": os.environ.get("SMOKE_ALL", "").split(),
            "NEW": os.environ.get("SMOKE_NEW", "").split()}

cmd = sys.argv[1]
if cmd == "split":
    split()
elif cmd in ("eval", "val"):
    c = ctx()
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
elif cmd == "compare":
    # compare <기준 파일> <컬렉션,컬렉션> <설정도 비교 0|1> : 지금 본문(state)이 기준 파일과 같은지
    ref = load(sys.argv[2])
    colls = [x for x in sys.argv[3].split(",") if x]
    try:
        st = json.loads(read("body", "rb").decode("utf-8"))
    except Exception:
        print("본문이 JSON 이 아님")
        sys.exit(1)
    diff = [k for k in colls if not isinstance(st.get(k), list) or norm(ref.get(k)) != norm(st.get(k))]
    if sys.argv[4] == "1" and json.dumps(ref.get("settings"), sort_keys=True) != json.dumps(st.get("settings"), sort_keys=True):
        diff.append("settings")
    if diff:
        print("다른 곳: " + ", ".join(diff))
        sys.exit(1)
elif cmd == "patched":
    # patched <원래 doc 파일> <patch 파일> : 본문이 '원래 + patch, null 키는 삭제' 와 같은지
    doc, patch = load(sys.argv[2]), load(sys.argv[3])
    d = json.loads(read("body", "rb").decode("utf-8") or "null")
    if not isinstance(d, dict):
        print("본문이 객체가 아님"); sys.exit(1)
    want = dict(doc)
    for k, v in patch.items():
        if v is None:
            want.pop(k, None)
        else:
            want[k] = v
    probs = [k for k, v in want.items() if d.get(k) != v]
    probs += [k + "(남아 있음)" for k, v in patch.items() if v is None and k in d]
    if probs:
        print("다른 키: " + ", ".join(probs)); sys.exit(1)
elif cmd == "make_v1":
    # make_v1 <2.0 export> <출력> : 1.0 내보내기 모양(notes·blockers·fridays·settings만, 메모는 1개만)
    src = load(sys.argv[2])
    out = {"format": src.get("format"), "version": "1.0.0", "exportedAt": src.get("exportedAt"),
           "settings": src.get("settings"), "notes": (src.get("notes") or [])[:1],
           "blockers": src.get("blockers") or [], "fridays": src.get("fridays") or []}
    with open(sys.argv[3], "w", encoding="utf-8") as f:
        json.dump(out, f, ensure_ascii=False)
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

# expect <파이썬 식> <설명>
#   식에서 쓸 수 있는 이름: d(JSON), t(본문 글), h(헤더, 소문자 키), s(상태), hall(), re, json, env,
#   load(파일), unfold(), events(), prop(), script_srcs(), norm(), ALL, NEW
expect() {
  local expr=$1 desc=$2 out
  if out=$(py eval "$expr" 2>&1); then ok "$desc"; else bad "$desc" "$out"; fi
}
soft() {
  local expr=$1 desc=$2 out
  if out=$(py eval "$expr" 2>&1); then ok "$desc"; else note "$desc" "$out"; fi
}
# same_as <기준 파일> <컬렉션 csv> <설정 0|1> <설명>  — 지금 본문(state)을 기준 파일과 비교
same_as() {
  local out
  if out=$(py compare "$1" "$2" "$3" 2>&1); then ok "$4"; else bad "$4" "$out"; fi
}

# 변수가 든 JSON 은 $( ) 밖에서 만듭니다(macOS bash 3.2 는 $( ) 안의 "{...,...}" 를 중괄호 확장함).
json_file() { printf '%s' "$2" >"$T/$1"; printf '%s' "$T/$1"; }

# 2.0 컬렉션 시험용 doc 과 patch (SPEC2 §4 모양, 일반 낱말만).
# patch 의 "tmp":null 은 그 키를 지우라는 뜻입니다.
write_samples() {
  local c=$1 doc patch
  case $c in
    requests)
      doc='{"raw":"'"$SECRET_MARK"' 요청 원문","what":"요청 하나","from":"다른 파트 개발자","program":"","size":"S","why":"","when":"","check":"","urgent":false,"status":"접수","receivedAt":"2026-10-19T09:00:00+09:00","askedAt":null,"forwardedAt":null,"closedAt":null,"absorbed":false,"absorbedAt":null,"est":null,"note":"","createdAt":"2026-10-19T09:01:00+09:00","tmp":"지울 값"}'
      patch='{"status":"정리 중","askedAt":"2026-10-19T10:00:00+09:00","tmp":null}' ;;
    programs)
      doc='{"order":1,"name":"프로그램 1","system":"","lead":"p1","sub":"p2","active":true,"subImpl":false,"brief":{"problem":"","success":"","constraints":"","out":""},"note":"","tmp":"지울 값"}'
      patch='{"active":false,"note":"고친 메모","tmp":null}' ;;
    decisions)
      doc='{"date":"2026-10-19","text":"결정 하나","why":"이유","by":"파트 리드","program":"","tmp":"지울 값"}'
      patch='{"why":"고친 이유","tmp":null}' ;;
    mapitems)
      doc='{"item":"항목 하나","program":"","status":"진행","risk":"","decide":"","done":false,"updatedAt":"2026-10-19T09:00:00+09:00","tmp":"지울 값"}'
      patch='{"done":true,"updatedAt":"2026-10-20T09:00:00+09:00","tmp":null}' ;;
    timelog)
      doc='{"date":"2026-10-19","kind":"접수·명확화·회신","hours":1.5,"tmp":"지울 값"}'
      patch='{"hours":2,"tmp":null}' ;;
    weeks)
      doc='{"signals":{"reviewEmpty":false,"offMap":false},"note":"","tmp":"지울 값"}'
      patch='{"signals":{"reviewEmpty":true,"offMap":false},"tmp":null}' ;;
    cycles)
      doc='{"checks":{"0":true,"1":false},"tmp":"지울 값"}'
      patch='{"checks":{"0":true,"1":true},"tmp":null}' ;;
    risks)
      doc='{"code":"R1","title":"리스크 하나","signal":"신호","response":"대응","level":"낮음","checkedAt":null,"note":"","order":1,"tmp":"지울 값"}'
      patch='{"level":"주의","checkedAt":"2026-10-19","tmp":null}' ;;
    *) die "모르는 컬렉션: $c" ;;
  esac
  printf '%s' "$doc" >"$T/$c.doc.json"
  printf '%s' "$patch" >"$T/$c.patch.json"
}
# 남겨 둘 doc 의 id (weeks·cycles 는 그 주 월요일)
keep_id() {
  case $1 in
    weeks|cycles) printf '2026-10-19' ;;
    *) printf 'smoke-keep-%s' "$1" ;;
  esac
}

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
printf 'manager-notebook 2.1 smoke — 모드: %s\n' "$MODE"

if [[ $MODE == docker ]]; then
  if [[ $BUILD == 1 ]]; then
    step "이미지 빌드: $IMAGE"
    if [[ -n $EXPECTED_VERSION ]]; then
      docker build --build-arg "VERSION=$EXPECTED_VERSION" -t "$IMAGE" "$ROOT_DIR"
    else
      docker build -t "$IMAGE" "$ROOT_DIR"
    fi
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
  if [[ -n $EXPECTED_VERSION ]]; then
    expect "(d.get('Labels') or {}).get('org.opencontainers.image.version') == env['EXPECTED_VERSION']" "OCI 라벨 version = $EXPECTED_VERSION"
  fi
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
HEALTH_VERSION=$(val "d.get('version','') if isinstance(d, dict) else ''")
export HEALTH_VERSION
if [[ -n $EXPECTED_VERSION ]]; then
  expect "d.get('version') == env['EXPECTED_VERSION']" "버전 = $EXPECTED_VERSION"
else
  soft "str(d.get('version','')).startswith('2.')" "버전이 2.x ($HEALTH_VERSION) — 정확히 보려면 EXPECTED_VERSION 을 주세요"
fi

# ---------------------------------------------------------------- 정적 파일
step "정적 파일"
for spec in "/|text/html" "/core.js|text/javascript" "/app.js|javascript" "/app.css|text/css" "/content.json|json" "/favicon.svg|image/svg+xml" "/login|text/html"; do
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
    /content.json)
      expect "isinstance(d, dict) and all(k in d for k in ('app','intake','prompts'))" "  content.json 이 JSON 이고 2.0 기본 키(app·intake·prompts)가 있음"
      soft "(d.get('app') or {}).get('title_ko') == '관리 수첩 2'" "  app.title_ko = '관리 수첩 2'"
      ;;
    /)
      expect "(lambda ss: [x[0].lstrip('/').lstrip('./') for x in ss if x[0].lstrip('/').lstrip('./') in ('core.js','app.js')] == ['core.js','app.js'])(script_srcs(t))" "  index.html 이 core.js → app.js 순서로 불러옴"
      expect "all(x[1] for x in script_srcs(t) if x[0].lstrip('/').lstrip('./') in ('core.js','app.js'))" "  core.js·app.js 모두 defer"
      ;;
  esac
  if [[ $path == / || $path == /login || $path == /app.css || $path == /app.js || $path == /core.js ]]; then
    expect "not re.search(r'(?i)(<script[^>]+src=[\"\\']?https?:|<link[^>]+href=[\"\\']?https?:|@import\\s+url\\(\\s*[\"\\']?https?:|url\\(\\s*[\"\\']?https?:)', t)" "  외부에서 불러오는 스크립트·스타일·글꼴 없음"
    expect "not re.search(r'(?i)fonts\\.googleapis|fonts\\.gstatic|googletagmanager|google-analytics|cdnjs|jsdelivr|unpkg', t)" "  CDN·글꼴·분석 주소 없음"
    if [[ $path == / || $path == /login ]]; then
      expect "not re.search(r'(?is)<script(?![^>]*\\bsrc=)[^>]*>\\s*\\S', t)" "  인라인 스크립트 없음(CSP)"
      expect "not re.search(r'(?i)\\sstyle\\s*=', t)" "  인라인 style 속성 없음(CSP)"
    fi
  fi
  if [[ $path == /content.json || $path == / || $path == /app.js || $path == /core.js ]]; then
    soft "'claude.ai' not in t" "  'claude.ai' 문구 없음"
  fi
done

# ---------------------------------------------------------------- 보안 헤더
step "보안 헤더"
CSP="default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data:; connect-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'"
export SMOKE_CSP="$CSP"
for path in / /core.js /app.js /api/health /api/state /api/guide /api/nope; do
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
step "처음 상태(11개 컬렉션)와 LLM 꺼짐"
req GET /api/state
expect_status 200 "GET /api/state"
expect "isinstance(d, dict) and all(k in d for k in ('version','settings','llm'))" "state 에 version·settings·llm"
expect "isinstance(d, dict) and all(isinstance(d.get(k), list) for k in ALL) and len(ALL) == 11" "state 에 컬렉션 키 11개가 모두 배열로 있음"
expect "isinstance(d, dict) and [k for k in ALL if k not in d] == []" "  빠진 컬렉션 키 없음 (빠진 것: $(val "[k for k in ALL if not isinstance(d, dict) or k not in d]"))"
if [[ $MODE == docker ]]; then
  expect "all(d.get(k) == [] for k in ALL)" "새 볼륨에서는 11개 모두 빈 배열"
  soft "d.get('settings') is None" "새 볼륨에서는 settings = null"
fi
expect "d['llm'].get('enabled') is False" "llm.enabled = false (LLM_BASE_URL 없음)"

req POST /api/llm "$(json_file llm.json '{"messages":[{"role":"user","content":"안녕"}]}')"
expect_status 404 "POST /api/llm (꺼짐)"
expect "d.get('error') == 'llm_disabled' and d.get('message')" "오류 본문 {error:llm_disabled, message}"

req POST /api/login "$(json_file login0.json '{"password":"x"}')"
expect_status 404 "비밀번호 없을 때 POST /api/login 은 404"

# ---------------------------------------------------------------- 1.0 컬렉션 CRUD
step "1.0 컬렉션 CRUD (notes·blockers·fridays)"
json_file n1.json "$(printf '{"kind":"q","text":"%s 물음 하나","createdAt":"2026-10-19T09:10:00+09:00","done":false}' "$SECRET_MARK")" >/dev/null
req POST /api/docs/notes "$T/n1.json"
expect_status 201 "POST /api/docs/notes"
expect "isinstance(d, dict) and isinstance(d.get('id'), str) and d['id'] and d.get('kind') == 'q'" "저장된 doc 에 id 가 있음"
N1=$(val "d.get('id','') if isinstance(d, dict) else ''")

req POST /api/docs/notes "$(json_file n2.json '{"kind":"l","text":"둘째","createdAt":"2026-10-19T09:11:00+09:00","done":false}')"
N2=$(val "d.get('id','') if isinstance(d, dict) else ''")
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

req PUT "/api/docs/notes/bad%20id%21" "$(json_file o2.json '{"a":1}')"
expect "400 <= s < 500" "id 형식이 틀리면 4xx ($(status))"

req PUT /api/docs/notes/smoke-put "$(json_file n4.json '{"kind":"q","text":"교체 전","createdAt":"2026-10-19T09:13:00+09:00","done":false,"old":1}')"
req PUT /api/docs/notes/smoke-put "$(json_file n5.json '{"kind":"q","text":"교체 후","createdAt":"2026-10-19T09:13:00+09:00","done":false}')"
expect_status 200 "PUT 전체 교체"
expect "d.get('text') == '교체 후' and 'old' not in d" "PUT 은 병합이 아니라 교체"

req DELETE "/api/docs/notes/$N2"
expect_status 204 "DELETE /api/docs/notes/<id>"

req POST /api/docs/blockers "$(json_file b1.json '{"kind":"b","text":"막힘 하나","receivedAt":"2026-10-19T17:00:00+09:00","state":"open","pausedDays":0,"pausedAt":null,"wasPaused":false,"resolvedAt":null,"createdAt":"2026-10-19T17:01:00+09:00","program":"smoke-keep-programs"}')"
expect_status 201 "POST /api/docs/blockers"

# fridays: 2.0 기록(v:2) 하나와 1.0 기록(v 없음) 하나
req PUT /api/docs/fridays/2026-10-19 "$(json_file f1.json '{"v":2,"week":"2026-10-19","draft":"이번 주 보고 초안","map":[{"item":"항목 하나","program":"","status":"진행","risk":"","decide":""}],"counts":{"waiting":0,"over":0,"absorbed":0,"active":1},"savedAt":"2026-10-23T15:00:00+09:00"}')"
expect_status 200 "PUT /api/docs/fridays/2026-10-19 (2.0 기록)"
req PUT /api/docs/fridays/2026-10-12 "$(json_file f0.json '{"week":"2026-10-12","fields":{"done":["끝난 일"],"risks":[],"decisions":[],"scope":[],"remaining":[],"gap":""},"team":[{"line":"끝난 일","source":"done"}],"boss5":[],"company":"","kept_line":"","savedAt":"2026-10-16T15:00:00+09:00"}')"
expect_status 200 "PUT /api/docs/fridays/2026-10-12 (1.0 모양 기록)"

# ---------------------------------------------------------------- 2.0 컬렉션 CRUD
step "2.0 컬렉션 8개: POST · PATCH(null 삭제) · DELETE"
for c in $NEW_COLLS; do
  write_samples "$c"
  req POST "/api/docs/$c" "$T/$c.doc.json"
  expect_status 201 "POST /api/docs/$c"
  cid=$(val "d.get('id','') if isinstance(d, dict) else ''")
  if [[ -z $cid ]]; then
    bad "$c: POST 응답에 id 가 없음" "$(head -c 200 "$T/body")"
    continue
  fi
  req PATCH "/api/docs/$c/$cid" "$T/$c.patch.json"
  expect_status 200 "PATCH /api/docs/$c/<id>"
  if out=$(py patched "$T/$c.doc.json" "$T/$c.patch.json" 2>&1); then ok "  $c: 병합되고 null 키(tmp)는 지워짐"; else bad "  $c: PATCH 결과가 다름" "$out"; fi
  export SMOKE_CID="$cid" SMOKE_C="$c"
  req GET /api/state
  expect "any(x.get('id') == env['SMOKE_CID'] and 'tmp' not in x for x in d.get(env['SMOKE_C'], []))" "  state.$c 에 고친 doc 이 보임"
  req DELETE "/api/docs/$c/$cid"
  expect_status 204 "DELETE /api/docs/$c/<id>"
  req GET /api/state
  expect "not any(x.get('id') == env['SMOKE_CID'] for x in d.get(env['SMOKE_C'], []))" "  지운 doc 은 state.$c 에 없음"
  # 내보내기·데이터 유지 시험용으로 하나 남겨 둡니다.
  req PUT "/api/docs/$c/$(keep_id "$c")" "$T/$c.doc.json"
  expect_status 200 "PUT /api/docs/$c/$(keep_id "$c") (남겨 둘 것)"
done

req PATCH /api/docs/requests/does-not-exist "$(json_file p3.json '{"status":"진행"}')"
expect_status 404 "2.0 컬렉션에서도 없는 doc PATCH 는 404"

for c in other settings meta docs; do
  req PUT "/api/docs/$c/x" "$(json_file o1.json '{"a":1}')"
  expect_status 404 "허용하지 않는 컬렉션 PUT /api/docs/$c/x 는 404"
done
req POST /api/docs/other "$(json_file o3.json '{"a":1}')"
expect_status 404 "허용하지 않는 컬렉션 POST /api/docs/other 는 404"

# ---------------------------------------------------------------- 설정
step "설정(1.0 키 + 2.0 키)"
req PUT /api/settings "$(json_file s1.json '{"startDate":"2026-10-19","cycleStartWeek":2,"cycleWeeks":6,"cooldownWeeks":1,"holidays":"2026-12-25 성탄절\n2027-01-01 새해","companyTemplate":"실적\n계획\n이슈","people":[{"id":"p1","label":"A","leadCap":2},{"id":"p2","label":"B","leadCap":2},{"id":"p3","label":"C","leadCap":2},{"id":"p4","label":"D","leadCap":2}],"slotCap":2,"requesterRoles":["다른 파트 개발자","기타"],"urgentCriteria":["서비스가 멈춤"],"bounds":null,"seededRisks":true,"onboarded":true}')"
expect_status 200 "PUT /api/settings"

req GET /api/state
expect "sorted(x['id'] for x in d['notes']) == sorted([env['SMOKE_N1'], 'smoke-patch.1', 'smoke-put'])" "state.notes 가 만든 것과 같음(지운 것은 없음)"
expect "len(d['blockers']) == 1 and sorted(x.get('id') for x in d['fridays']) == ['2026-10-12','2026-10-19']" "state.blockers 1개·fridays 2개"
expect "all(len(d[k]) == 1 for k in NEW)" "state 의 2.0 컬렉션마다 남겨 둔 doc 1개 ($(val "{k: len(d.get(k) or []) for k in NEW}"))"
expect "isinstance(d['settings'], dict) and all(d['settings'].get(k) == v for k, v in load('s1.json').items())" "state.settings 가 저장한 값과 같음(people·slotCap 등 2.0 키 포함)"
cp "$T/body" "$T/state0.json"

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
step "내보내기(11개 컬렉션) → 가져오기 왕복"
req GET /api/export
expect_status 200 "GET /api/export"
expect "re.fullmatch(r'attachment;\\s*filename=\"manager-notebook-\\d{8}\\.json\"', h.get('content-disposition',''))" "Content-Disposition: attachment; filename=\"manager-notebook-YYYYMMDD.json\""
expect "isinstance(d, dict) and d.get('format') == 'manager-notebook-export' and all(k in d for k in ('version','exportedAt','settings'))" "export 머리(format·version·exportedAt·settings)"
expect "isinstance(d, dict) and all(isinstance(d.get(k), list) for k in ALL)" "export 에 컬렉션 키 11개 (빠진 것: $(val "[k for k in ALL if not isinstance(d, dict) or not isinstance(d.get(k), list)]"))"
expect "d.get('version') == env['HEALTH_VERSION']" "export version = health version ($HEALTH_VERSION)"
same_as "$T/state0.json" "$ALL_CSV" 1 "export 내용이 state 와 같음(11개 + 설정)"
cp "$T/body" "$T/export.json"

# 가져오기 전에 데이터를 바꿔 둡니다.
req POST /api/docs/notes "$(json_file n6.json '{"kind":"q","text":"가져오기로 사라질 메모","createdAt":"2026-10-19T10:00:00+09:00","done":false}')"
req DELETE /api/docs/requests/smoke-keep-requests
req POST /api/docs/risks "$T/risks.doc.json"
req PUT /api/settings "$(json_file s2.json '{"startDate":"2027-01-04","cycleStartWeek":2,"cycleWeeks":6,"cooldownWeeks":1,"holidays":"","companyTemplate":""}')"

req POST /api/import "$T/export.json"
expect_status 200 "POST /api/import (2.0 파일)"
expect "isinstance(d, dict) and sorted(d.get('replaced') or []) == sorted(ALL)" "replaced 가 11개 모두 ($(val "d.get('replaced') if isinstance(d, dict) else d"))"
expect "isinstance(d, dict) and d.get('counts') == {k: len(load('export.json')[k]) for k in ALL}" "counts 가 파일의 개수와 같음"
expect "isinstance(d, dict) and d.get('settings') is True" "settings: true"
req GET /api/state
same_as "$T/export.json" "$ALL_CSV" 1 "가져온 뒤 state 가 내보낸 파일과 같음(모두 교체)"

# ---------------------------------------------------------------- 1.0 모양 파일 가져오기
step "1.0 내보내기 모양 가져오기(notes·blockers·fridays·settings만)"
py make_v1 "$T/export.json" "$T/v1.json"
req POST /api/import "$T/v1.json"
expect_status 200 "POST /api/import (1.0 파일)"
expect "isinstance(d, dict) and sorted(d.get('replaced') or []) == ['blockers','fridays','notes']" "replaced 가 notes·blockers·fridays 뿐 ($(val "d.get('replaced') if isinstance(d, dict) else d"))"
expect "isinstance(d, dict) and d.get('counts') == {k: len(load('v1.json')[k]) for k in ('notes','blockers','fridays')}" "counts 는 그 셋만"
expect "isinstance(d, dict) and d.get('settings') is True" "settings: true (파일에 settings 가 있음)"
req GET /api/state
same_as "$T/v1.json" "$OLD_CSV" 1 "notes·blockers·fridays·설정은 1.0 파일 내용으로 바뀜"
same_as "$T/export.json" "$NEW_CSV" 0 "2.0 컬렉션 8개는 그대로 남음"

req POST /api/import "$T/export.json"
expect_status 200 "2.0 파일로 다시 되돌림"

# ---------------------------------------------------------------- 틀린 가져오기
step "틀린 가져오기는 아무것도 바꾸지 않음"
req POST /api/import "$(json_file bad1.json '{"format":"something-else","notes":[],"requests":[]}')"
expect_status 400 "format 이 다르면 400"
req GET /api/state
same_as "$T/export.json" "$ALL_CSV" 1 "  데이터가 그대로"

req POST /api/import "$(json_file bad2.json '{"format":"manager-notebook-export","requests":[],"notes":"not-a-list"}')"
expect "400 <= s < 500" "컬렉션 값이 배열이 아니면 4xx ($(status))"
req GET /api/state
same_as "$T/export.json" "$ALL_CSV" 1 "  데이터가 그대로(한 트랜잭션: requests 도 비워지지 않음)"

# ---------------------------------------------------------------- 일부만 든 파일
step "일부 컬렉션만 든 파일(빈 배열은 비움, settings 없으면 그대로)"
req POST /api/import "$(json_file part.json '{"format":"manager-notebook-export","risks":[]}')"
expect_status 200 "POST /api/import {risks:[]}"
expect "isinstance(d, dict) and d.get('replaced') == ['risks'] and d.get('counts') == {'risks': 0}" "replaced=[risks], counts={risks:0}"
expect "isinstance(d, dict) and d.get('settings') is False" "settings: false (키가 없음)"
req GET /api/state
expect "d.get('risks') == []" "risks 가 비워짐"
same_as "$T/export.json" "$(csv $OLD_COLLS requests programs decisions mapitems timelog weeks cycles)" 1 "  나머지 10개와 설정은 그대로"

req POST /api/import "$T/export.json"
expect_status 200 "2.0 파일로 다시 되돌림"
req GET /api/state
same_as "$T/export.json" "$ALL_CSV" 1 "  되돌린 state 가 내보낸 파일과 같음"

# ---------------------------------------------------------------- ics
step "일정 파일(.ics 2.0)"
req GET /api/calendar.ics
expect_status 200 "GET /api/calendar.ics"
expect "h.get('content-type','').lower().startswith('text/calendar') and 'utf-8' in h.get('content-type','').lower()" "Content-Type text/calendar; charset=utf-8"
expect "'attachment' in h.get('content-disposition','') and 'manager-notebook.ics' in h.get('content-disposition','')" "첨부 파일 이름 manager-notebook.ics"
expect "t.startswith('BEGIN:VCALENDAR') and t.rstrip().endswith('END:VCALENDAR')" "VCALENDAR 로 감쌈"
expect "'BEGIN:VTIMEZONE' in t and 'TZID:Asia/Seoul' in unfold(t)" "VTIMEZONE Asia/Seoul"
expect "len(events(t)) >= 1" "VEVENT 가 1개 이상 ($(val "len(events(t))")개)"
if [[ -n $ICS_EXPECTED_EVENTS && $ICS_EXPECTED_EVENTS != 0 ]]; then
  export ICS_EXPECTED_EVENTS
  expect "len(events(t)) == int(env['ICS_EXPECTED_EVENTS'])" "VEVENT ${ICS_EXPECTED_EVENTS}개"
fi
expect "(lambda u: len(u) == len(events(t)) and len(u) == len(set(u)) and all(x.endswith('-v2@manager-notebook') for x in u))([l.split(':',1)[1].strip() for e in events(t) for l in prop(e,'UID')])" "UID 가 일정마다 하나, 겹치지 않고 '-v2@manager-notebook' 로 끝남"
expect "[e for e in events(t) if any(re.fullmatch(r'RRULE:FREQ=WEEKLY;BYDAY=FR;UNTIL=\\d{8}T\\d{6}Z', l) for l in prop(e,'RRULE'))]" "금요 읽기: RRULE:FREQ=WEEKLY;BYDAY=FR;UNTIL=…Z"
expect "any(prop(e,'EXDATE') for e in events(t) if prop(e,'RRULE'))" "금요 읽기에 EXDATE 가 있음(쉬는 날 금요일)"
expect "any('20261225' in l for e in events(t) if prop(e,'RRULE') for l in prop(e,'EXDATE'))" "  EXDATE 에 2026-12-25(금, 설정의 쉬는 날)가 들어 있음"
soft "any('20261023T135500' in l for e in events(t) if prop(e,'RRULE') for l in prop(e,'DTSTART'))" "  첫 금요 읽기가 2026-10-23 13:55"
expect "'RDATE' not in unfold(t)" "RDATE 없음(Outlook 호환)"
expect "not re.search(r'(?m)^RRULE:FREQ=MONTHLY;BYDAY=[^;]*,', unfold(t))" "서수 여러 개짜리 MONTHLY 규칙 없음(Outlook 호환)"
expect "[l for e in events(t) for l in prop(e,'SUMMARY')] and all(l.split(':',1)[1].startswith('[관리] ') for e in events(t) for l in prop(e,'SUMMARY'))" "모든 제목이 '[관리] '로 시작"
expect "all(len(l.encode('utf-8')) <= 75 for l in t.replace('\\r\\n','\\n').split('\\n'))" "모든 줄이 75옥텟 이하(접기)"
expect "'claude.ai' not in t" "ics 에 클라우드 링크 없음"
soft "unfold(t).count('BEGIN:VALARM') == len(events(t))" "일정마다 알림(VALARM) 1개"
soft "any('사이클 1 시작' in l for e in events(t) for l in prop(e,'SUMMARY'))" "'사이클 1 시작' 일정이 있음"
soft "'\\r\\n' in t" "줄 끝이 CRLF(RFC 5545)"
soft "'localhost:8080/#' in unfold(t)" "설명에 수첩 탭 링크(localhost:8080/#…)가 있음"

# ---------------------------------------------------------------- guide
step "안내"
req GET /api/guide
expect_status 200 "GET /api/guide"
expect "isinstance(d, list) and all(isinstance(x, dict) and x.get('slug') and x.get('title') and 'order' in x for x in d)" "목록 [{slug,title,order}]"
expect "isinstance(d, list) and [x.get('slug') for x in d] == ['start','raci','watch','templates']" "안내 4개 start·raci·watch·templates 순서 ($(val "[x.get('slug') for x in d] if isinstance(d, list) else d"))"
for slug in start raci watch templates; do
  req GET "/api/guide/$slug"
  expect_status 200 "GET /api/guide/$slug"
  expect "isinstance(d, dict) and d.get('slug') == '$slug' and d.get('title') and isinstance(d.get('html'), str) and len(d['html']) > 20" "  본문 {slug,title,html}"
  expect "'<script' not in d.get('html','').lower()" "  본문 HTML 에 script 없음(raw HTML 꺼짐)"
  if [[ $slug == start ]]; then
    soft "'href=\"#' in d.get('html','')" "  탭으로 가는 링크(#…)가 있음"
  fi
done
req GET /api/guide/no-such-guide-page
expect_status 404 "없는 안내는 404"

# ---------------------------------------------------------------- 로그
if [[ $MODE == docker ]]; then
  step "로그"
  docker logs "$APP" >"$T/applog" 2>&1 || true
  if grep -q "$SECRET_MARK" "$T/applog"; then bad "로그에 메모·요청 내용이 남음" "$(grep -m1 "$SECRET_MARK" "$T/applog")"; else ok "로그에 메모·요청 내용 없음"; fi
  if grep -q 'GET /api/health 200' "$T/applog"; then bad "정상 health 요청이 로그에 남음(HEALTHCHECK 30초마다 쌓임)"; else ok "정상 health 요청은 로그에 남기지 않음"; fi
  if grep -qiE 'https?://[^ ]*(googleapis|gstatic|github|docker|update)' "$T/applog"; then note "로그에 외부 주소가 보임" "$(grep -m1 -iE 'https?://' "$T/applog")"; else ok "로그에 외부 연결 흔적 없음"; fi
fi

# ---------------------------------------------------------------- 체험 모드
# --demo 는 메모리에 예시 데이터를 만들고 /data 볼륨을 건드리지 않아야 합니다.
# 바로 뒤 '인증 + 데이터 유지'에서 같은 볼륨이 그대로인지(export 와 같은지) 다시 확인합니다.
if [[ $MODE == docker ]]; then
  step "체험 모드(--demo): 메모리의 예시 데이터, 볼륨은 그대로"
  APP_DEMO="$APP-demo"
  docker rm -f "$APP_DEMO" "$APP_DEMO-curl" >/dev/null 2>&1 || true
  docker run -d --name "$APP_DEMO" --network none --read-only --cap-drop ALL \
    --security-opt no-new-privileges:true -v "$VOL:/data" "$IMAGE" --demo >/dev/null
  docker run -d --name "$APP_DEMO-curl" --network "container:$APP_DEMO" \
    --entrypoint sleep "$CURL_IMAGE" 1800 >/dev/null
  SAVED_HELPER=$HELPER
  HELPER="$APP_DEMO-curl"
  if wait_health "$APP_DEMO"; then
    req GET /api/state
    expect_status 200 "체험 모드 GET /api/state"
    expect "d.get('demo') is True" "state.demo = true"
    expect "len(d.get('programs') or []) == 7 and len(d.get('requests') or []) == 7" "예시 프로그램 7개·요청 7건"
    expect "(d.get('settings') or {}).get('people') and len(d['settings']['people']) == 4" "예시 사람 4명(A~D)"
  fi
  docker rm -f "$APP_DEMO" "$APP_DEMO-curl" >/dev/null 2>&1 || true
  HELPER=$SAVED_HELPER
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
  for path in / /login /core.js /app.js /app.css /content.json /favicon.svg; do
    req GET "$path"
    expect_status 200 "쿠키 없이 정적 파일 $path"
  done
  req GET /login
  expect "'text/html' in h.get('content-type','') and not re.search(r'(?is)<script(?![^>]*\\bsrc=)[^>]*>\\s*\\S', t) and not re.search(r'(?i)\\sstyle\\s*=', t)" "/login 은 HTML 이고 인라인 스크립트·style 없음"
  for spec in "GET /api/state" "GET /api/export" "GET /api/calendar.ics" "GET /api/guide" "PUT /api/settings" "POST /api/docs/notes" "POST /api/docs/requests" "PATCH /api/docs/programs/smoke-keep-programs" "DELETE /api/docs/risks/smoke-keep-risks" "POST /api/llm" "POST /api/import"; do
    m=${spec%% *}; p=${spec#* }
    if [[ $m == GET || $m == DELETE ]]; then req "$m" "$p"; else req "$m" "$p" "$(json_file empty.json '{}')"; fi
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
  same_as "$T/export.json" "$ALL_CSV" 1 "컨테이너를 지우고 다시 만들어도 11개 컬렉션·설정이 남음(볼륨)"
  req GET /api/calendar.ics
  expect_status 200 "쿠키로 GET /api/calendar.ics"
  req POST /api/docs/requests "$T/requests.doc.json"
  expect_status 201 "쿠키로 POST /api/docs/requests"

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
