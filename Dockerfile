# syntax=docker/dockerfile:1.7
#
# manager-notebook 2.0: 프로그램 리드 체계에서 쓰는 개인 관리 수첩(인터넷이 막힌 업무 PC용).
# 이 이미지는 공개(public) 레지스트리에 올라갑니다. 회사 이름, 사내 주소,
# 토큰, 사람 이름, 개인 정보는 이 파일과 이미지에 절대 넣지 않습니다.
# 사내 값은 실행할 때 --env-file 로만 넣습니다.
#
# 로컬 빌드:   docker build -t manager-notebook:dev .
# 버전 지정:   docker build --build-arg VERSION=2.1.0 -t manager-notebook:2.1.0 .
# 배포 빌드:   scripts/release.sh (buildx 멀티아키 + SBOM + provenance)

# SPEC §8 은 golang:1.24 였지만, 1.24 로 만든 바이너리는 trivy 에서 Go 표준 라이브러리
# HIGH·CRITICAL 22건이 나옵니다(2026-10 기준, 1.25.x/1.26.x 에서만 고쳐짐).
# SPEC §9-6 목표(HIGH 이상 0건)를 지키려고 지원 중인 1.26 을 기본으로 씁니다.
# go.mod 의 go 1.24.0 은 그대로 두어도 됩니다(최소 버전일 뿐).
# 베이스 이미지는 digest 로 고정합니다. 같은 소스면 언제 빌드해도 같은 베이스를 씁니다.
# 고칠 때는 digest 만 올립니다. 실행 단계(distroless)는 .github/dependabot.yml 이 제안하고,
# 빌드 단계(golang, ARG 라서 자동 제안이 안 될 수 있음)는 아래 명령으로 직접 확인합니다.
#   docker buildx imagetools inspect golang:1.26-bookworm   # Digest: 줄 (2026-10-22: 1.26.10)
# 버전을 바꾸려면: docker build --build-arg GO_VERSION=1.27 --build-arg GO_DIGEST= .
ARG GO_VERSION=1.26
ARG GO_DIGEST=@sha256:632dff7351799a5a9954a48dc06670ade72ee44e8c351cdc0e5a57af2fea21af

# ---------- 1단계: 빌드 ----------
# 빌드는 빌드하는 PC의 아키텍처에서 하고, Go 교차 컴파일로 대상 아키텍처 바이너리를 만듭니다.
# (QEMU 없이도 linux/amd64, linux/arm64 를 빠르게 만듭니다.)
FROM --platform=$BUILDPLATFORM golang:${GO_VERSION}-bookworm${GO_DIGEST} AS build

ARG TARGETOS
ARG TARGETARCH
ARG VERSION=2.1.0

ENV CGO_ENABLED=0 \
    GOFLAGS=-mod=readonly

WORKDIR /src

# 의존성 먼저 받아 두면 소스만 바뀔 때 다시 받지 않습니다.
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download && go mod verify

COPY . .

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH:-amd64} \
    go build -trimpath -buildvcs=false \
      -ldflags "-s -w -X main.version=${VERSION}" \
      -o /out/manager-notebook ./cmd/manager-notebook

# 비루트 사용자(65532)가 쓸 수 있는 빈 /data 를 만들어 둡니다.
# 새 이름 있는 볼륨은 처음 붙을 때 이 폴더의 소유자를 그대로 받습니다.
RUN mkdir -p /out/data && chmod 0750 /out/data

# ---------- 2단계: 실행 ----------
# distroless static: 셸·패키지 관리자가 없고, 기본 사용자가 nonroot(65532)입니다.
# digest 는 nonroot 태그의 멀티아키 목록(index)입니다.
FROM gcr.io/distroless/static-debian12:nonroot@sha256:afa5c872c891853ca7fcf1f12c3edb23f7eeef36189728842dd51042ff57f7ab

ARG VERSION=2.1.0

LABEL org.opencontainers.image.title="manager-notebook" \
      org.opencontainers.image.description="프로그램 리드 체계용 개인 관리 수첩. 외부 연결 없음, 데이터는 /data 볼륨에만 저장." \
      org.opencontainers.image.source="https://github.com/jameskwon07/manager-notebook" \
      org.opencontainers.image.licenses="MIT" \
      org.opencontainers.image.version="${VERSION}"

COPY --from=build /out/manager-notebook /manager-notebook
COPY --from=build --chown=65532:65532 /out/data /data

# 업무 PC에서 저장소를 받을 수 없을 때를 위해 안내 파일을 이미지 안에 함께 둡니다.
# 꺼내는 법: docker create --name nb-tmp <이미지> ; docker cp nb-tmp:/usr/share/doc/manager-notebook/notebook.env.example notebook.env ; docker rm -v nb-tmp
COPY README.md LICENSE notebook.env.example /usr/share/doc/manager-notebook/

ENV TZ=Asia/Seoul \
    NOTEBOOK_DATA_DIR=/data \
    NOTEBOOK_ADDR=:8080

USER nonroot

EXPOSE 8080
VOLUME ["/data"]

# distroless 에는 셸·curl 이 없어서 바이너리의 healthcheck 하위 명령을 씁니다.
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
  CMD ["/manager-notebook", "healthcheck"]

ENTRYPOINT ["/manager-notebook"]
