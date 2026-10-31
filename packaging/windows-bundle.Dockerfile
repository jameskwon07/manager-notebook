# syntax=docker/dockerfile:1.7
#
# Windows 실행 파일을 레지스트리로 옮기기 위한 묶음 이미지입니다. 실행하는 이미지가 아닙니다.
# 인터넷이 막힌 PC에서 레지스트리만 열려 있을 때 이렇게 꺼냅니다.
#   docker pull ghcr.io/jameskwon07/manager-notebook:<버전>-windows
#   docker create --name nb-exe ghcr.io/jameskwon07/manager-notebook:<버전>-windows x
#   docker cp nb-exe:/manager-notebook-<버전>-windows-amd64.zip .
#   docker cp nb-exe:/SHA256SUMS.txt .
#   docker rm nb-exe
# 빌드 맥락은 scripts/build-windows.sh 가 만든 dist 폴더입니다.
FROM scratch
ARG VERSION=2.1.0
LABEL org.opencontainers.image.title="manager-notebook (Windows 실행 파일 묶음)" \
      org.opencontainers.image.description="관리 수첩 Windows 실행 파일 zip. 실행하는 이미지가 아니라 docker cp 로 꺼내 쓰는 묶음입니다." \
      org.opencontainers.image.source="https://github.com/jameskwon07/manager-notebook" \
      org.opencontainers.image.licenses="MIT" \
      org.opencontainers.image.version="${VERSION}"
COPY *.zip SHA256SUMS.txt README-windows.txt /
