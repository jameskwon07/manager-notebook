# 관리 수첩 (manager-notebook)

인터넷이 막힌 업무 PC에서 쓰는 개인 관리 수첩입니다.
물음 메모, 막힘 시계(근무일 기준), 금요일 요약 초안, 상황별 할 말, 실행 가이드를 한 화면에 모았습니다.
Docker 이미지 하나로 돌고, 브라우저에서 `http://localhost:8080` 으로 엽니다.

## 약속

- **외부 연결이 없습니다.** CDN, 웹 글꼴, 분석 도구, 업데이트 확인을 쓰지 않습니다.
  예외는 하나입니다. 사내 AI 주소(`LLM_BASE_URL`)를 직접 적었을 때만, 버튼을 누르면 그 주소로 요청을 보냅니다.
- **데이터는 볼륨에만 있습니다.** `manager-notebook` 볼륨 안의 SQLite 파일 하나(`notebook.db`)입니다.
  컨테이너를 지워도 볼륨은 남습니다.
- **이미지에 회사·개인 정보가 없습니다.** 누구나 받을 수 있는 공개 이미지입니다.
  회사 주소, 비밀번호, 키는 실행할 때 `notebook.env` 파일로만 넣습니다.
- 팀원 개인을 세거나 평가하는 기능이 없습니다. 자동으로 보내는 기능도 없습니다. 모두 복사해서 씁니다.
- Jira·Confluence·Synology Chat 과 연동하지 않습니다. 복사해서 붙여 넣기만 합니다.

## 이미지 주소

두 곳에 같은 이미지를 올립니다. 업무망에서 받을 수 있는 쪽을 쓰세요.

| 레지스트리 | 주소 |
|---|---|
| GitHub (GHCR) | `ghcr.io/jameskwon07/manager-notebook:1.0.0` |
| Docker Hub | `docker.io/<DOCKERHUB_USER>/manager-notebook:1.0.0` |

- `<DOCKERHUB_USER>` 는 배포한 사람의 Docker Hub 이름으로 바꿔 읽으세요.
- **버전 태그(`1.0.0`)를 쓰세요.** `latest` 도 있지만, 언제 바뀔지 모릅니다.
- 회사가 레지스트리 미러(프록시)를 쓰면 주소 앞부분이 다를 수 있습니다. 예: `미러주소/jameskwon07/manager-notebook:1.0.0`. IT 부서에 물어보세요.
- linux/amd64, linux/arm64 를 모두 지원합니다.

## 준비물

- Windows 10/11 업무 PC
- 아래 둘 중 하나
  - **Docker Desktop**
    - 직원 250명 이상이거나 연 매출 1천만 달러 이상인 회사는 **유료 구독**이 필요합니다.
    - 회사가 구독이 있는지 먼저 확인하세요.
  - **Rancher Desktop** (무료, 오픈소스)
    - 설치할 때 컨테이너 엔진을 **dockerd (moby)** 로 고르세요.
    - 설정 → Kubernetes → **Enable Kubernetes 를 끄세요.** 수첩에는 필요 없고, 메모리를 많이 씁니다.
    - 아래 `docker` 명령을 그대로 쓰도록 만들었지만, Rancher Desktop 에서는 아직 시험하지 못했습니다.
      처음 실행 5번의 확인 명령으로 꼭 확인하세요.
- 둘 다 WSL2 를 씁니다. 설치에 관리자 권한이나 IT 승인이 필요할 수 있습니다.

## 처음 실행 (Windows PowerShell)

PowerShell 을 열고 한 줄씩 붙여 넣습니다.

**1. 이미지 받기** (둘 중 하나)

```powershell
docker pull ghcr.io/jameskwon07/manager-notebook:1.0.0
# 또는
docker pull docker.io/<DOCKERHUB_USER>/manager-notebook:1.0.0
```

이 뒤로는 받은 주소를 `$IMG` 에 넣고 씁니다. PowerShell 창을 새로 열면 이 줄을 다시 붙여 넣으세요.
아래 실행 명령은 설정 파일을 `"$HOME\manager-notebook\notebook.env"` 처럼 전체 경로로 찾습니다. 그래서 어느 폴더에서 붙여 넣어도 됩니다.

```powershell
$IMG = "ghcr.io/jameskwon07/manager-notebook:1.0.0"
```

**2. 데이터 볼륨 만들기** (한 번만)

```powershell
docker volume create manager-notebook
```

**3. 설정 파일 만들기** (한 번만)

설정 파일은 내 폴더에 둡니다. 아무 값이 없어도 동작합니다.

```powershell
New-Item -ItemType Directory -Force "$HOME\manager-notebook" | Out-Null
cd "$HOME\manager-notebook"
Set-Content -Encoding ascii notebook.env "TZ=Asia/Seoul"
```

설명이 달린 예시 파일이 필요하면 이미지 안에서 꺼냅니다. **이미 고친 `notebook.env` 가 있으면 덮어씁니다.**

```powershell
cd "$HOME\manager-notebook"
docker create --name nb-tmp $IMG
docker cp nb-tmp:/usr/share/doc/manager-notebook/notebook.env.example notebook.env
docker rm -v nb-tmp
notepad notebook.env
```

- 메모장에서 저장할 때 인코딩은 **UTF-8** 로 둡니다.
- 값에 따옴표를 쓰지 마세요. 따옴표도 값이 됩니다.
- 값 뒤에 빈칸을 남기지 마세요. 빈칸도 값이 됩니다(특히 비밀번호).

**4. 실행**

```powershell
docker run -d --name manager-notebook --restart unless-stopped -p 127.0.0.1:8080:8080 -v manager-notebook:/data --env-file "$HOME\manager-notebook\notebook.env" $IMG
```

**5. 열기**

브라우저에서 <http://localhost:8080> 을 엽니다.

먼저 PowerShell 에서 확인해도 됩니다. `{"ok":true,...}` 가 나오면 정상입니다.

```powershell
curl.exe http://127.0.0.1:8080/api/health
```

- `-p 127.0.0.1:8080:8080` 으로 열었기 때문에 **내 PC에서만** 열립니다. 다른 PC에서는 열리지 않습니다.
- **이 `127.0.0.1` 이 수첩의 실제 보호막입니다.** `-p 8080:8080` 처럼 바꿔 다른 PC에서 열지 마세요. 비밀번호를 켜도 연결은 암호화되지 않습니다.
- `--restart unless-stopped` 덕분에 Docker 가 켜지면 수첩도 같이 켜집니다.
  Docker Desktop(또는 Rancher Desktop)의 '로그인할 때 시작' 설정을 켜 두세요.
- 8080 포트를 이미 쓰고 있으면, 실패한 컨테이너가 남아 있으니 먼저 `docker rm -f manager-notebook` 을 합니다.
  그다음 `-p 127.0.0.1:18080:8080` 으로 바꿔 다시 실행하고 <http://localhost:18080> 을 엽니다.
  이때 일정 파일(.ics) 설명 안의 수첩 링크는 8080 그대로라서 열리지 않습니다.

**더 단단하게 (선택)**

아래 옵션을 붙여도 동작합니다. 쓰기는 `/data` 에만 합니다. 로그 파일 크기도 30MB 안으로 묶습니다.

```powershell
docker run -d --name manager-notebook --restart unless-stopped -p 127.0.0.1:8080:8080 -v manager-notebook:/data --env-file "$HOME\manager-notebook\notebook.env" --read-only --cap-drop ALL --security-opt no-new-privileges:true --log-opt max-size=10m --log-opt max-file=3 $IMG
```

## 설정 바꾸기

`notebook.env` 를 고친 뒤에는 컨테이너를 지우고 다시 만듭니다. 데이터는 볼륨에 있어서 그대로입니다.

```powershell
docker rm -f manager-notebook
docker run -d --name manager-notebook --restart unless-stopped -p 127.0.0.1:8080:8080 -v manager-notebook:/data --env-file "$HOME\manager-notebook\notebook.env" $IMG
```

`docker restart` 만으로는 바뀐 설정이 들어가지 않습니다.

## 비밀번호 켜기

같은 PC를 다른 사람도 쓴다면 켜 두세요.

1. `notebook.env` 에 한 줄을 넣습니다. **8자 이상**으로 적으세요(짧으면 시작 로그에 경고가 남습니다).
   ```
   NOTEBOOK_PASSWORD=여기에-긴-비밀번호
   ```
2. 위 '설정 바꾸기' 대로 다시 만듭니다.
3. 이제 처음 열 때 로그인 화면이 나옵니다. 로그인은 12시간 유지됩니다.

- 끄려면 그 줄을 지우고 다시 만듭니다.
- 비밀번호를 잊으면 `notebook.env` 에서 새로 적고 다시 만듭니다.
- `notebook.env` 는 내 폴더에만 두고 공유하지 마세요.
- 비밀번호는 같은 PC의 다른 사람을 막는 용도입니다. 연결을 암호화하지 않으니, 다른 PC에서 열도록 바꾸지 마세요.

## 사내 AI 켜기 (선택)

기본은 꺼짐입니다. 꺼져 있어도 모든 기능이 틀(템플릿)로 동작합니다.
켜면 막힘 회신, 할 말, 금요일 초안에 '사내 AI로 다듬기' 버튼이 생깁니다.

> 사내 AI 에 보내도 되는 내용인지 회사 규정을 먼저 확인하세요.
> 버튼을 누를 때만 보냅니다. 저절로 보내지 않습니다.

1. `notebook.env` 에 적습니다. 주소는 OpenAI 호환(`/v1/chat/completions` 를 받는 곳)이어야 합니다.
   ```
   LLM_BASE_URL=https://llm.example.internal/v1
   LLM_API_KEY=사내에서-받은-키
   LLM_MODEL=사내-모델-이름
   ```
   - 주소 끝은 `/v1` 까지만 씁니다. `/chat/completions` 까지 붙여 넣었다면 수첩이 잘라 내고 시작 로그에 알려 줍니다.
   - 주소는 `https://` 를 쓰세요. `http://` 에 키를 넣으면 키가 암호화 없이 가서 시작 로그에 경고가 남습니다.
   - 사내 게이트웨이가 추가 헤더를 요구하면 `LLM_EXTRA_HEADERS=이름1=값1;이름2=값2` 를 씁니다.
   - 응답이 느리면 `LLM_TIMEOUT_SEC=120` 처럼 늘립니다(기본 90초).
2. '설정 바꾸기' 대로 다시 만듭니다.
3. 설정 탭에서 '사내 AI: 켜짐' 과 모델 이름을 확인합니다.

**사내 인증서가 필요할 때**

사내 AI 주소가 회사 자체 인증서를 쓰면 연결이 실패합니다. 인증서 파일(PEM)을 넣어 줍니다.

1. 인증서 파일을 예를 들어 `C:\certs\company-ca.pem` 에 둡니다.
   - Windows 에서 꺼낼 때: `certmgr.msc` → 회사 루트 인증서 → 모든 작업 → 내보내기 → **Base 64로 인코딩된 X.509(.CER)** 를 고릅니다.
   - 메모장으로 열어 첫 줄이 `-----BEGIN CERTIFICATE-----` 인지 봅니다. 아니면(DER) 수첩이 읽지 못합니다.
   - 파일이 먼저 있어야 합니다. 없는 경로를 `-v` 로 붙이면 Docker 가 같은 이름의 빈 폴더를 만듭니다.
2. `notebook.env` 에 적습니다.
   ```
   SSL_CERT_FILE=/certs/company-ca.pem
   ```
3. 실행할 때 `-v` 를 하나 더 붙입니다.
   ```powershell
   docker rm -f manager-notebook
   docker run -d --name manager-notebook --restart unless-stopped -p 127.0.0.1:8080:8080 -v manager-notebook:/data -v C:\certs\company-ca.pem:/certs/company-ca.pem:ro --env-file "$HOME\manager-notebook\notebook.env" $IMG
   ```
4. `docker logs --tail 50 manager-notebook` 에서 `사내 인증서 1개 읽음` 을 확인합니다. 못 읽으면 `경고: SSL_CERT_FILE(...)에서 인증서를 못 읽었습니다` 가 보입니다.

`C:\` 경로를 `-v` 로 붙이는 것이 안 되면(Rancher Desktop 등), Windows 경로를 쓰지 않는 방법이 있습니다. 인증서를 볼륨에 한 번 복사해 둡니다.

```powershell
docker volume create manager-notebook-certs
docker create --name nb-cert -v manager-notebook-certs:/certs busybox:1.37
docker cp C:\certs\company-ca.pem nb-cert:/certs/
docker rm -v nb-cert
```

그다음 실행 명령의 `-v C:\certs\company-ca.pem:/certs/company-ca.pem:ro` 대신 `-v manager-notebook-certs:/certs:ro` 를 붙입니다.

`LLM_INSECURE_SKIP_VERIFY=true` 는 인증서 확인을 끄는 마지막 수단입니다. 켜면 시작할 때 경고가 남습니다. 위 방법을 먼저 쓰세요.

## 업데이트

1. 먼저 백업합니다(아래 '백업').
2. 새 버전을 받습니다.
   ```powershell
   $IMG = "ghcr.io/jameskwon07/manager-notebook:1.0.1"
   docker pull $IMG
   ```
3. 컨테이너만 바꿉니다. 데이터는 볼륨에 남습니다.
   ```powershell
   docker rm -f manager-notebook
   docker run -d --name manager-notebook --restart unless-stopped -p 127.0.0.1:8080:8080 -v manager-notebook:/data --env-file "$HOME\manager-notebook\notebook.env" $IMG
   ```
4. 문제가 있으면 이전 버전 태그로 다시 만듭니다. 새 버전이 데이터 형식을 바꿨다면 이전 버전이 못 열 수 있습니다. 그래서 1번 백업이 필요합니다.

## 백업과 복원

**쉬운 방법: 설정 탭**

- '데이터 내보내기(.json)' 를 누르면 파일 하나로 받습니다.
- 되살릴 때는 '가져오기' 를 씁니다. 지금 데이터를 **모두 바꿉니다**. 두 번 눌러 확인합니다.

**볼륨 통째로**

이 이미지에는 셸이 없습니다(distroless). 그래서 공개 `busybox` 이미지로 압축합니다.
잠깐 멈추고 하면 파일이 깨지지 않습니다.

```powershell
cd "$HOME\manager-notebook"
docker stop manager-notebook
docker run --rm -v manager-notebook:/data:ro -v "${PWD}:/backup" busybox:1.37 tar czf /backup/manager-notebook-backup.tgz -C /data .
docker start manager-notebook
```

되살리기:

```powershell
cd "$HOME\manager-notebook"
docker stop manager-notebook
docker run --rm -v manager-notebook:/data -v "${PWD}:/backup" busybox:1.37 sh -c "rm -rf /data/* && tar xzf /backup/manager-notebook-backup.tgz -C /data && chown -R 65532:65532 /data"
docker start manager-notebook
```

내보낸 파일과 백업 파일에도 메모가 그대로 들어 있습니다. 안전한 곳에 두세요.

## 일정 파일(.ics)

- 설정 탭의 '내 업무 캘린더용 일정 파일 받기(.ics)' 를 누릅니다.
- **'관리 수첩' 캘린더를 따로 만들어 넣기를 권합니다.** 그만둘 때 그 캘린더만 지우면 됩니다.
  - 클래식 Outlook: .ics 파일을 두 번 눌러 '새로 열기' 를 고르면 따로 생깁니다.
  - 새 Outlook·웹: '캘린더 추가 → 파일에서 업로드' 에서 새로 만든 캘린더를 고릅니다(메뉴 이름은 확인 필요).
  - 그룹웨어: 캘린더의 '가져오기' 를 씁니다.
- 일정 제목은 모두 `[관리]` 로 시작합니다.
- **시작일이나 쉬는 날을 바꿨다면, 먼저 지난번 `[관리]` 일정(따로 만든 캘린더면 그 캘린더)을 모두 지운 뒤** 새 파일을 가져옵니다.
  시작일이 바뀌면 일정 이름표(UID)가 모두 바뀌고, Outlook 가져오기는 같은 이름표도 덮어쓰지 않고 복사본을 만들 수 있습니다. 지우지 않으면 알림이 두 번씩 뜹니다.
- 쉬는 날은 설정 탭의 '쉬는 날' 목록으로 뺍니다. 첫째 월요일이 쉬는 날이면 1:1 안내는 다음 근무일에 따로 한 번 뜹니다.
- 가져온 뒤 한 번 확인하세요(기본 시작일 2026-10-19 기준).
  - 11-09·11-16(월) 09:20 에 `[관리] 막힘 확인` 이 있습니다.
  - 12-07(첫째 월요일)에는 `막힘 확인` 이 없고 `[관리] 막힘 확인 + 이달 1:1 안내` 만 있습니다.
  - 2027-03-02(화) 09:20 에 1:1 안내가 있습니다(03-01 삼일절에서 옮김).

## 지우기

```powershell
docker rm -f manager-notebook
docker volume rm manager-notebook
docker image rm $IMG
```

- `docker volume rm` 은 **모든 메모를 영구히 지웁니다.** 되돌릴 수 없습니다. 필요하면 먼저 내보내세요.
- `notebook.env` 와 백업 파일은 직접 지웁니다.
- 캘린더에 넣은 일정은 '관리 수첩' 캘린더를 지우거나, `[관리]` 로 시작하는 일정을 지우면 됩니다.

## 이미지 검증

받은 이미지가 배포한 것과 같은지 확인할 수 있습니다. 인터넷이 되는 PC에서 해도 됩니다.

**digest 보기**

```powershell
docker buildx imagetools inspect ghcr.io/jameskwon07/manager-notebook:1.0.0
docker buildx imagetools inspect docker.io/<DOCKERHUB_USER>/manager-notebook:1.0.0
```

- 두 곳의 `Digest:` 값이 같아야 합니다.
- digest 로 받으면 태그가 바뀌어도 항상 같은 이미지를 받습니다.
  ```powershell
  docker pull ghcr.io/jameskwon07/manager-notebook@sha256:<digest>
  ```
- 이미 받은 이미지의 digest: `docker image inspect $IMG --format '{{json .RepoDigests}}'`

**SBOM(들어 있는 부품 목록) 보기**

```powershell
docker buildx imagetools inspect ghcr.io/jameskwon07/manager-notebook:1.0.0 --format '{{json .SBOM}}'
```

**빌드 기록(provenance) 보기**

```powershell
docker buildx imagetools inspect ghcr.io/jameskwon07/manager-notebook:1.0.0 --format '{{json .Provenance}}'
```

**취약점 검사(선택, 인터넷이 되는 PC에서)**

배포할 때마다 HIGH·CRITICAL 0건을 확인한 뒤 올립니다. 직접 해 보려면 검사기 이미지를 digest 로 고정해 받고,
Docker 소켓 대신 이미지 파일(tar)만 읽기 전용으로 넘깁니다.

```powershell
cd "$HOME\manager-notebook"
docker save $IMG -o image.tar
docker run --rm -v "${PWD}:/scan:ro" aquasec/trivy:0.65.0@sha256:a22415a38938a56c379387a8163fcb0ce38b10ace73e593475d3658d578b2436 image --input /scan/image.tar --severity HIGH,CRITICAL
Remove-Item image.tar
```

## 문제 해결

| 증상 | 확인할 것 |
|---|---|
| 페이지가 안 열림 | `docker ps` 에 `manager-notebook` 이 있는지, `docker logs --tail 50 manager-notebook` 에 오류가 있는지 |
| `port is already allocated` | 8080 을 다른 프로그램이 씁니다. 먼저 `docker rm -f manager-notebook` 을 한 뒤 `-p 127.0.0.1:18080:8080` 으로 다시 실행합니다 |
| `open ...notebook.env: no such file` | 설정 파일 경로가 틀렸습니다. `"$HOME\manager-notebook\notebook.env"` 가 있는지 봅니다(처음 실행 3번) |
| `docker pull` 실패 | 회사 미러 주소나 프록시가 필요한지 IT 에 묻습니다. 다른 레지스트리 주소로도 해 봅니다 |
| 상태가 `unhealthy` | `docker logs --tail 50 manager-notebook` 과 `docker inspect --format "{{json .State.Health}}" manager-notebook` 을 봅니다. 그래도 모르면 '설정 바꾸기' 대로 다시 만듭니다 |
| `/data` 쓰기 권한 오류 | 폴더를 직접 붙이지 말고(`-v C:\...:/data`), 이름 있는 볼륨(`-v manager-notebook:/data`)을 쓰세요 |
| 사내 AI 버튼이 안 보임 | 설정 탭의 사내 AI 상태, `LLM_BASE_URL` 철자, 다시 만들었는지 |
| 사내 AI 오류(502, 504) | `docker logs --tail 50 manager-notebook` 의 `llm connect error: <원인> (<호스트>)` 줄을 봅니다. `cert` 는 사내 인증서, `dns` 는 주소 철자, `proxy` 는 프록시, `refused` 는 주소·포트입니다. `llm upstream status 401` 처럼 숫자가 있으면 키·모델 이름·사내 AI 쪽 상태를 확인합니다. `3xx` 는 주소가 다른 곳으로 넘기는 것이니 `LLM_BASE_URL` 을 확인합니다(수첩은 따라가지 않습니다) |
| 사내 AI 에 프록시가 필요함 | `notebook.env` 에 `HTTPS_PROXY=...` 와 `NO_PROXY=...` 를 적습니다. 수첩은 이 표준 변수를 따릅니다. 반대로 사내 AI 가 프록시를 타면 안 되면 `NO_PROXY=사내AI호스트` 를 적습니다 |
| `403` `host` 오류 | <http://localhost:8080> (포트를 바꿨으면 그 포트)으로 여세요. 다른 PC에서 여는 것은 권하지 않습니다 |

## 설정 값 전체

| 이름 | 기본값 | 뜻 |
|---|---|---|
| `NOTEBOOK_ADDR` | `:8080` | 컨테이너 안에서 듣는 주소. 바꾸지 마세요(상태 확인이 8080 을 봅니다) |
| `NOTEBOOK_DATA_DIR` | `/data` | SQLite 파일(`notebook.db`) 위치 |
| `NOTEBOOK_PASSWORD` | (없음) | 적으면 로그인 화면이 켜짐. 8자 이상 |
| `TZ` | `Asia/Seoul` | 시간대 |
| `LLM_BASE_URL` | (없음) | 사내 AI 주소. 비우면 AI 꺼짐 |
| `LLM_API_KEY` | (없음) | `Authorization: Bearer` 로 보냄 |
| `LLM_MODEL` | (없음) | 모델 이름 |
| `LLM_TIMEOUT_SEC` | `90` | 응답 제한 시간(초) |
| `LLM_EXTRA_HEADERS` | (없음) | `이름1=값1;이름2=값2` 추가 헤더 |
| `SSL_CERT_FILE` | (없음) | 사내 인증서 파일 경로(볼륨으로 넣음) |
| `LLM_INSECURE_SKIP_VERIFY` | `false` | 인증서 확인 끄기. 마지막 수단 |

## 개발과 배포 (배포하는 사람용)

```bash
docker build -t manager-notebook:dev .        # 빌드
BUILD=1 scripts/smoke.sh                       # 빌드 + 연기 시험(--network none --read-only)
scripts/release.sh 1.0.0 <DOCKERHUB_USER>      # 검사 후 두 레지스트리에 멀티아키 push
```

- **가이드 공개 확인.** `guide/*.md` 와 `web/content.json` 은 공개 이미지에 들어가고, 이미지 주소·라벨의 GitHub 이름과 함께 보입니다.
  첫 push 전에 상사·팀의 개인 사정, 사내 이름처럼 공개하면 안 되는 문장이 없는지 읽어 봅니다.
  `release.sh` 가 이 확인을 묻습니다(이미 확인했으면 `GUIDE_PUBLIC_OK=1`).

- `release.sh` 는 로그인하지 않습니다. 먼저 `docker login docker.io`, `docker login ghcr.io` 를 해 두세요.
- push 전에 smoke 시험과 trivy 검사(HIGH·CRITICAL 0건)를 통과해야 합니다. trivy 이미지는 digest 로 고정하고, Docker 소켓 대신 `docker save` 한 tar 를 검사합니다.
- 베이스 이미지(Dockerfile)와 GitHub 액션(release.yml)은 digest·커밋 SHA 로 고정했습니다. 올릴 때는 `.github/dependabot.yml` 의 제안을 봅니다.
- GitHub 에 `v1.0.0` 같은 태그를 올리면 `.github/workflows/release.yml` 이 같은 일을 합니다.
  Docker Hub 에도 올리려면 저장소 Secrets 에 `DOCKERHUB_USERNAME`, `DOCKERHUB_TOKEN` 을 넣습니다.
- **GHCR 패키지는 처음 올리면 비공개입니다.** GitHub → Packages → manager-notebook → Package settings → Change visibility 에서 Public 으로 바꿔야 로그인 없이 받을 수 있습니다.
- Docker Hub 저장소도 Public 인지 확인합니다.

## 라이선스

MIT. [LICENSE](LICENSE) 를 보세요.
