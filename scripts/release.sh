#!/usr/bin/env bash
# manager-notebook 배포 스크립트 — 두 공개 레지스트리(Docker Hub, GHCR)에 멀티아키 이미지를 올립니다.
#
# 쓰는 법
#   scripts/release.sh <VERSION> <DOCKERHUB_USER>
#   VERSION=1.0.0 DOCKERHUB_USER=<DOCKERHUB_USER> scripts/release.sh
#
# 선택 환경변수
#   GHCR_REPO=ghcr.io/jameskwon07/manager-notebook   GHCR 이미지 이름
#   SKIP_DOCKERHUB=1      Docker Hub 는 건너뛰고 GHCR 에만 올림(DOCKERHUB_USER 불필요)
#   SKIP_GHCR=1           GHCR 은 건너뛰고 Docker Hub 에만 올림
#   SKIP_SMOKE=1          push 전 scripts/smoke.sh 를 건너뜀
#   NO_LATEST=1           latest 태그를 붙이지 않음
#   DRY_RUN=1             검사까지만 하고 push 명령은 출력만 함
#   TRIVY_IMAGE=aquasec/trivy:<ver>@sha256:<digest>   trivy 바이너리가 없을 때 쓸 공개 이미지(digest 고정)
#   GUIDE_PUBLIC_OK=1     가이드 글이 공개된다는 확인을 묻지 않음(이미 확인했을 때)
#   TRIVY_IGNORE_UNFIXED=1             고칠 버전이 아직 없는 취약점은 세지 않음
#   PLATFORMS=linux/amd64,linux/arm64
#
# 이 스크립트는 레지스트리에 로그인하지 않습니다.
# 먼저 직접 로그인해 두세요(토큰은 스크립트에 넣지 않습니다).
#   docker login docker.io -u <DOCKERHUB_USER>        # 비밀번호 대신 Docker Hub 액세스 토큰
#   docker login ghcr.io -u <GitHub 사용자>          # write:packages 권한이 있는 토큰
#
# 순서: 입력 확인 → 로그인 확인 → buildx 준비 → 로컬 빌드 → smoke → trivy → 멀티아키 push → 결과 확인

set -euo pipefail

SCRIPT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
ROOT_DIR=$(cd "$SCRIPT_DIR/.." && pwd)

VERSION=${1:-${VERSION:-}}
DOCKERHUB_USER=${2:-${DOCKERHUB_USER:-}}
GHCR_REPO=${GHCR_REPO:-ghcr.io/jameskwon07/manager-notebook}
PLATFORMS=${PLATFORMS:-linux/amd64,linux/arm64}
BUILDER=${BUILDER:-manager-notebook-builder}
# 태그만으로 받지 않고 digest 로 고정합니다(스캐너 이미지 바꿔치기 방지). 올릴 때 버전과 digest 를 함께 바꿉니다.
TRIVY_IMAGE=${TRIVY_IMAGE:-aquasec/trivy:0.65.0@sha256:a22415a38938a56c379387a8163fcb0ce38b10ace73e593475d3658d578b2436}
GUIDE_PUBLIC_OK=${GUIDE_PUBLIC_OK:-0}
SKIP_DOCKERHUB=${SKIP_DOCKERHUB:-0}
SKIP_GHCR=${SKIP_GHCR:-0}
SKIP_SMOKE=${SKIP_SMOKE:-0}
NO_LATEST=${NO_LATEST:-0}
DRY_RUN=${DRY_RUN:-0}
TRIVY_IGNORE_UNFIXED=${TRIVY_IGNORE_UNFIXED:-0}

say() { printf '\n== %s\n' "$*"; }
die() { printf 'release: %s\n' "$*" >&2; exit 1; }

# ---------------------------------------------------------------- 1. 입력 확인
say "입력 확인"
[[ -n $VERSION ]] || die "VERSION 이 없습니다. 예: scripts/release.sh 1.0.0 <DOCKERHUB_USER>"
VERSION=${VERSION#v}
[[ $VERSION =~ ^[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$ ]] || die "VERSION 은 1.0.0 같은 형식이어야 합니다: $VERSION"

if [[ $SKIP_DOCKERHUB != 1 ]]; then
  [[ -n $DOCKERHUB_USER ]] || die "DOCKERHUB_USER 가 없습니다. 두 번째 인자로 주거나 SKIP_DOCKERHUB=1 을 쓰세요."
  [[ $DOCKERHUB_USER =~ ^[a-z0-9]+([._-][a-z0-9]+)*$ ]] || die "DOCKERHUB_USER 형식이 이상합니다(소문자·숫자): $DOCKERHUB_USER"
  [[ $DOCKERHUB_USER != "<DOCKERHUB_USER>" ]] || die "DOCKERHUB_USER 자리표시자를 실제 이름으로 바꾸세요."
fi
[[ $SKIP_DOCKERHUB == 1 && $SKIP_GHCR == 1 ]] && die "SKIP_DOCKERHUB 와 SKIP_GHCR 를 둘 다 켜면 올릴 곳이 없습니다."

GHCR_REPO=$(printf '%s' "$GHCR_REPO" | tr '[:upper:]' '[:lower:]')
HUB_REPO="docker.io/${DOCKERHUB_USER}/manager-notebook"

TAGS=()
if [[ $SKIP_DOCKERHUB != 1 ]]; then
  TAGS+=("$HUB_REPO:$VERSION")
  [[ $NO_LATEST == 1 ]] || TAGS+=("$HUB_REPO:latest")
fi
if [[ $SKIP_GHCR != 1 ]]; then
  TAGS+=("$GHCR_REPO:$VERSION")
  [[ $NO_LATEST == 1 ]] || TAGS+=("$GHCR_REPO:latest")
fi

command -v docker >/dev/null || die "docker 가 필요합니다."
docker info >/dev/null 2>&1 || die "Docker 엔진에 연결할 수 없습니다."
docker buildx version >/dev/null 2>&1 || die "docker buildx 가 필요합니다."

cd "$ROOT_DIR"
for f in Dockerfile go.mod go.sum assets.go cmd/manager-notebook/main.go web/index.html web/content.json guide/index.json LICENSE README.md notebook.env.example; do
  [[ -e $f ]] || die "필요한 파일이 없습니다: $f"
done
# SPEC §2: 다른 담당이 만든 자리표시 파일은 첫 줄에 PLACEHOLDER 가 있습니다. 남아 있으면 배포하지 않습니다.
PLACEHOLDERS=$(find web guide cmd internal assets.go Dockerfile README.md -type f -exec awk 'FNR==1 && /PLACEHOLDER/ {print FILENAME}' {} + 2>/dev/null || true)
[[ -z $PLACEHOLDERS ]] || die "자리표시(PLACEHOLDER) 파일이 남아 있습니다: $(printf '%s' "$PLACEHOLDERS" | tr '\n' ' ')"

# 가이드 공개 확인: guide/*.md 와 web/content.json 은 이미지에 들어가 누구나 읽을 수 있고,
# 이미지 주소·라벨에 GitHub 이름이 함께 보입니다. 개인 사정이 드러나는 문장이 없는지 직접 확인합니다.
if [[ $GUIDE_PUBLIC_OK != 1 && $DRY_RUN != 1 ]]; then
  printf '가이드 공개 확인: guide/*.md, web/content.json 은 공개 이미지에 들어가 GitHub 이름과 함께 보입니다.
'
  printf '  상사·팀의 개인 사정, 사내 이름, 채널 이름 등 공개하면 안 되는 문장이 없는지 읽어 보았나요?
'
  if [[ -t 0 ]]; then
    read -r -p '  확인했으면 y: ' ans
    [[ $ans == y || $ans == Y ]] || die "가이드 공개 확인을 하지 않아 멈춥니다."
  else
    die "가이드 공개 확인이 필요합니다. 확인했으면 GUIDE_PUBLIC_OK=1 을 붙여 다시 실행하세요."
  fi
fi

if command -v git >/dev/null && git -C "$ROOT_DIR" rev-parse --is-inside-work-tree >/dev/null 2>&1; then
  if [[ -n $(git -C "$ROOT_DIR" status --porcelain) ]]; then
    printf '주의: 커밋하지 않은 변경이 있습니다. 배포 이미지는 지금 폴더 내용으로 만들어집니다.\n'
  fi
  GIT_REVISION=$(git -C "$ROOT_DIR" rev-parse HEAD 2>/dev/null || echo "")
else
  GIT_REVISION=""
fi

printf '버전: %s\n' "$VERSION"
printf '아키텍처: %s\n' "$PLATFORMS"
printf '태그:\n'; printf '  %s\n' "${TAGS[@]}"

# ---------------------------------------------------------------- 2. 로그인 확인(로그인은 하지 않음)
say "레지스트리 로그인 확인(로그인은 하지 않습니다)"
DOCKER_CONFIG_FILE="${DOCKER_CONFIG:-$HOME/.docker}/config.json"
logged_in() {
  # config.json 의 auths 나 credHelpers 에 그 레지스트리가 있는지만 봅니다(토큰은 읽지 않음).
  local registry=$1
  [[ -f $DOCKER_CONFIG_FILE ]] || return 1
  python3 - "$DOCKER_CONFIG_FILE" "$registry" <<'PY'
import json, sys
cfg = json.load(open(sys.argv[1]))
reg = sys.argv[2]
names = set((cfg.get("auths") or {}).keys()) | set((cfg.get("credHelpers") or {}).keys())
aliases = {
    "docker.io": {"docker.io", "index.docker.io", "https://index.docker.io/v1/", "https://index.docker.io/v1", "registry-1.docker.io"},
    "ghcr.io": {"ghcr.io", "https://ghcr.io"},
}[reg]
sys.exit(0 if names & aliases else 1)
PY
}
command -v python3 >/dev/null || die "python3 가 필요합니다."
if [[ $SKIP_DOCKERHUB != 1 ]]; then
  logged_in docker.io || die "Docker Hub 에 로그인되어 있지 않습니다. 먼저: docker login docker.io -u $DOCKERHUB_USER"
  printf 'Docker Hub: 로그인 기록 있음\n'
fi
if [[ $SKIP_GHCR != 1 ]]; then
  logged_in ghcr.io || die "GHCR 에 로그인되어 있지 않습니다. 먼저: docker login ghcr.io -u <GitHub 사용자>"
  printf 'GHCR: 로그인 기록 있음\n'
fi
printf '(로그인 기록만 봅니다. 토큰이 만료됐으면 push 단계에서 실패합니다.)\n'

# ---------------------------------------------------------------- 3. buildx 빌더
say "buildx 빌더 준비: $BUILDER"
# 멀티아키 + SBOM·provenance 첨부에는 docker-container 드라이버 빌더가 필요합니다.
if ! docker buildx inspect "$BUILDER" >/dev/null 2>&1; then
  docker buildx create --name "$BUILDER" --driver docker-container >/dev/null
fi
docker buildx inspect --bootstrap "$BUILDER" >/dev/null

# ---------------------------------------------------------------- 4. 검사용 로컬 빌드
LOCAL_TAG="manager-notebook:release-check-$VERSION"
HOST_ARCH=$(docker version --format '{{.Server.Arch}}' 2>/dev/null || echo amd64)
say "검사용 로컬 빌드: $LOCAL_TAG (linux/$HOST_ARCH)"
docker buildx build \
  --builder "$BUILDER" \
  --platform "linux/$HOST_ARCH" \
  --build-arg "VERSION=$VERSION" \
  --load \
  -t "$LOCAL_TAG" \
  .

# ---------------------------------------------------------------- 5. smoke
if [[ $SKIP_SMOKE != 1 ]]; then
  say "smoke 시험"
  EXPECTED_VERSION="$VERSION" "$SCRIPT_DIR/smoke.sh" "$LOCAL_TAG" || die "smoke 시험이 실패했습니다. push 하지 않습니다."
else
  say "smoke 시험 건너뜀(SKIP_SMOKE=1)"
fi

# ---------------------------------------------------------------- 6. trivy
say "trivy 취약점 검사(HIGH, CRITICAL 이 0건이어야 push)"
TRIVY_ARGS=(image --scanners vuln --severity HIGH,CRITICAL --exit-code 1 --no-progress)
[[ $TRIVY_IGNORE_UNFIXED == 1 ]] && TRIVY_ARGS+=(--ignore-unfixed)
if command -v trivy >/dev/null; then
  trivy "${TRIVY_ARGS[@]}" "$LOCAL_TAG" || die "trivy 가 HIGH 이상 취약점을 찾았습니다. push 하지 않습니다."
else
  printf 'trivy 바이너리가 없어 %s 이미지로 검사합니다.\n' "$TRIVY_IMAGE"
  # docker.sock 을 넘기지 않습니다. docker save 한 tar 만 읽기 전용으로 넘깁니다.
  SCAN_DIR=$(mktemp -d)
  trap 'rm -rf "$SCAN_DIR"' EXIT
  docker save "$LOCAL_TAG" -o "$SCAN_DIR/image.tar"
  docker run --rm \
    -v "$SCAN_DIR:/scan:ro" \
    -v manager-notebook-trivy-cache:/root/.cache/ \
    "$TRIVY_IMAGE" "${TRIVY_ARGS[@]}" --input /scan/image.tar \
    || die "trivy 가 HIGH 이상 취약점을 찾았거나 검사에 실패했습니다. push 하지 않습니다."
  rm -rf "$SCAN_DIR"
fi

# ---------------------------------------------------------------- 7. 멀티아키 push
say "멀티아키 빌드와 push ($PLATFORMS)"
CREATED=$(date -u +%Y-%m-%dT%H:%M:%SZ)
BUILD_ARGS=(
  buildx build
  --builder "$BUILDER"
  --platform "$PLATFORMS"
  --build-arg "VERSION=$VERSION"
  --sbom=true
  --provenance=mode=max
  --label "org.opencontainers.image.created=$CREATED"
  --annotation "index:org.opencontainers.image.title=manager-notebook"
  --annotation "index:org.opencontainers.image.description=인터넷이 막힌 PC에서 쓰는 개인 관리 수첩. 외부 연결 없음, 데이터는 /data 볼륨에만 저장."
  --annotation "index:org.opencontainers.image.source=https://github.com/jameskwon07/manager-notebook"
  --annotation "index:org.opencontainers.image.licenses=MIT"
  --annotation "index:org.opencontainers.image.version=$VERSION"
)
if [[ -n $GIT_REVISION ]]; then
  BUILD_ARGS+=(--label "org.opencontainers.image.revision=$GIT_REVISION")
fi
for t in "${TAGS[@]}"; do BUILD_ARGS+=(-t "$t"); done
BUILD_ARGS+=(--push .)

if [[ $DRY_RUN == 1 ]]; then
  printf 'DRY_RUN=1: 아래 명령을 실행하지 않았습니다(인자를 한 줄에 하나씩 보여 줌).\n  docker\n'
  printf '    %s\n' "${BUILD_ARGS[@]}"
  exit 0
fi
docker "${BUILD_ARGS[@]}"

# ---------------------------------------------------------------- 8. 결과 확인
say "올라간 이미지 확인"
FIRST_TAG=${TAGS[0]}
DIGEST=$(docker buildx imagetools inspect "$FIRST_TAG" --format '{{json .Manifest}}' | python3 -c 'import json,sys; print(json.load(sys.stdin)["digest"])')
printf 'digest: %s\n' "$DIGEST"
for t in "${TAGS[@]}"; do
  d=$(docker buildx imagetools inspect "$t" --format '{{json .Manifest}}' | python3 -c 'import json,sys; print(json.load(sys.stdin)["digest"])')
  if [[ $d == "$DIGEST" ]]; then printf '  같은 digest: %s\n' "$t"; else die "digest 가 다릅니다: $t ($d)"; fi
done
docker buildx imagetools inspect "$FIRST_TAG" --format '{{json .Manifest}}' \
  | python3 -c 'import json,sys; m=json.load(sys.stdin); print("아키텍처:", ", ".join(sorted({p["platform"]["os"]+"/"+p["platform"]["architecture"] for p in m.get("manifests",[]) if p.get("platform",{}).get("os")!="unknown"})))'
SBOM_JSON=$(docker buildx imagetools inspect "$FIRST_TAG" --format '{{json .SBOM}}' 2>/dev/null || true)
PROV_JSON=$(docker buildx imagetools inspect "$FIRST_TAG" --format '{{json .Provenance}}' 2>/dev/null || true)
if [[ $SBOM_JSON == *SPDX* ]]; then
  printf 'SBOM: 있음\n'
else
  printf '주의: SBOM 을 찾지 못했습니다.\n'
fi
if [[ $PROV_JSON == *SLSA* || $PROV_JSON == *buildType* ]]; then
  printf 'provenance: 있음\n'
else
  printf '주의: provenance 를 찾지 못했습니다.\n'
fi

cat <<EOF

끝났습니다. 다음을 직접 확인하세요.
  1. GHCR 패키지는 처음 올리면 비공개(private)입니다.
     GitHub → 내 프로필 → Packages → manager-notebook → Package settings → Change visibility → Public
     (공개로 바꿔야 업무망에서 로그인 없이 pull 할 수 있습니다.)
  2. Docker Hub 저장소가 Public 인지 확인합니다(hub.docker.com → 저장소 → Settings).
  3. 업무 PC 에서는 버전 태그나 digest 로 받으세요.
     docker pull $FIRST_TAG
     docker pull ${FIRST_TAG%:*}@$DIGEST
EOF
