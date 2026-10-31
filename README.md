# 관리 수첩 2 (manager-notebook 2.1)

프로그램마다 리드 1명과 서브 1명을 두는 팀에서, 파트 리드가 혼자 쓰는 관리 수첩입니다.
외부 요청 접수, 프로그램·자리 계산, 결정 경계, 금요일 보고 초안, 물음 메모·막힘을 한 화면에 모았습니다.
인터넷이 막힌 업무 PC에서 Docker 이미지 하나, 또는 Docker 없이 Windows 실행 파일 하나로 돌고, 브라우저에서 `http://localhost:8080` 으로 엽니다.
먼저 둘러보려면 체험 모드(`--demo`)로 예시 데이터를 열어 보세요.

## 약속

- **외부 연결이 없습니다.** CDN, 웹 글꼴, 분석 도구, 업데이트 확인을 쓰지 않습니다.
  예외는 하나입니다. 사내 AI 주소(`LLM_BASE_URL`)를 직접 적었을 때만, 버튼을 누르면 그 주소로 요청을 보냅니다.
- **데이터는 이 PC의 파일 하나에만 있습니다.** SQLite 파일 하나(`notebook.db`)입니다.
  Docker 판은 볼륨 안에 두고(컨테이너를 지워도 볼륨은 남음), 실행 파일 판은 `%LOCALAPPDATA%\manager-notebook` 에 둡니다.
- **이미지에 회사·개인 정보가 없습니다.** 누구나 받을 수 있는 공개 이미지입니다.
  회사 주소, 비밀번호, 키는 실행할 때 `notebook.env` 파일로만 넣습니다.
  사람은 기본으로 `A`·`B`·`C`·`D` 기호로 표시합니다. 바꿀 때도 이니셜이나 기호만 쓰기를 권합니다.
- 팀원 개인을 평가하거나 순위를 매기는 기능이 없습니다. 사람별로 세는 것은 '자리 계산'(한 사람이 맡은 활성 자리 수가 상한을 넘는지) 한 곳뿐입니다.
- 자동으로 보내는 기능이 없습니다. 문장과 표는 모두 복사해서 씁니다.
- Jira·Confluence·사내 메신저와 연동하지 않습니다. 복사해서 붙여 넣기만 합니다.

## 2.1에서 바뀐 것

- **Docker 없이 쓰는 Windows 실행 파일**이 생겼습니다. 아래 'Windows 실행 파일로 쓰기'를 보세요.
- **체험 모드(`--demo`)**: 예시 데이터로 열어 보는 모드입니다. 기록은 메모리에만 있고 끄면 사라집니다. 쓰던 데이터는 건드리지 않습니다.
  - 실행 파일: `run-demo.cmd` 를 두 번 누르거나 `manager-notebook.exe --demo`
  - Docker: `docker run --rm -p 127.0.0.1:8090:8080 ghcr.io/jameskwon07/manager-notebook:2.1.0 --demo` 뒤 `http://localhost:8090`
- **요청 접수함만 쓰는 판(`--only intake`)**: 기능 하나만 남긴 화면입니다. 아래 '요청 접수함만 쓰기'를 보세요.
- 화면과 데이터 형식은 2.0과 같습니다. 2.0에서 그대로 올리면 됩니다.

## 요청 접수함만 쓰기 (한 기능판)

수첩의 기능 가운데 가장 많이 쓰는 하나만 남긴 화면입니다. 받은 요청을 그대로 붙여 넣고, 7칸 가운데 빈칸만 되묻고, 근무일 2일 안에 담당 리드에게 넘기는 일만 합니다.

| 켜는 법 | 명령 |
|---|---|
| 실행 파일 | `run-intake.cmd` 를 두 번 누르거나 `manager-notebook.exe --only intake` (체험은 `run-intake-demo.cmd`) |
| Docker | 실행 명령 끝에 `--only intake` 를 붙이거나 `-e NOTEBOOK_ONLY=intake` |
| 전체 수첩을 쓰는 중 | `http://localhost:8080/intake` |

- 화면은 셋입니다: **요청 적기**(받은 말 그대로 + 아는 칸), **받은 요청**(전달 시계·빈칸·상태와 [되묻기 문장 복사]·[넘길 문장 복사 · 리드 전달로]), **설정**(프로그램 목록·요청자 역할·쉬는 날·내보내기·가져오기, 접어 둠).
- 기록은 전체 수첩과 같습니다(요청·프로그램 목록·설정). 접수함만 쓰다가 전체 수첩으로 열어도 그대로 보입니다.
- 되묻기 문장을 복사하면 상태가 '정리 중'으로, 넘길 문장을 복사하면 '리드 전달'로 바뀌고 전달 시계가 멈춥니다. 잘못 눌렀으면 상태를 되돌리면 됩니다.

## 2.0에서 바뀐 것

화면을 처음부터 다시 짰습니다. 제목은 `관리 수첩 2`, 탭은 8개입니다.

| 탭 | 하는 일 |
|---|---|
| 오늘 | 숫자 타일 6개, 지금 손댈 것 목록, 막힘(근무일 시계), 조기 신호 6개, 물음 메모 빠른 추가 |
| 접수함 | 외부 요청 적기(7칸), 빈칸 되묻기 문장 복사, 리드에게 넘길 문장 복사, 전달 시계(근무일 2일), 흡수 표시, 회신 문장 6개 |
| 프로그램·자리 | 사람마다 리드·서브 구현 자리 계산과 상한 경고, 프로그램 표와 지침 네 칸(문제·성공 기준·제약·빼는 것), 사이클 시작 체크 |
| 결정 경계 | 누가 무엇을 정하는지 표(고칠 수 있음), 긴급 기준, 결정 로그. Confluence 표로 복사 |
| 금요일 | 진행 지도(위험·결정 필요 칸), 주간 서면 보고 초안, 이번 주 저장 |
| 수첩 | 물음 메모·나중에 보낼 것, 시간 일지, 리스크 표, 1.0 금요일 기록(읽기 전용) |
| 안내 | 쓰는 법, 역할과 책임(RACI), 조심할 것, 양식과 문장 |
| 설정 | 시작일·사이클·쉬는 날, 사람 표시·자리 상한, 일정 파일(.ics), 내보내기·가져오기, 사내 AI 상태 |

그 밖에 바뀐 것:

- **1.0 데이터를 그대로 엽니다.** 같은 볼륨으로 2.0을 띄우면 1.0의 물음 메모·막힘이 그대로 보이고, 1.0 금요일 기록은 수첩 탭의 '1.0 기록'에 보입니다.
- **가져오기**는 파일에 들어 있는 항목만 바꿉니다. 2.0 내보내기 파일은 모두 바꾸고, 1.0 내보내기 파일은 물음 메모·막힘·금요일·설정만 바꾸고 2.0에서 새로 생긴 기록은 그대로 둡니다.
- **일정 파일(.ics)** 내용이 바뀌었습니다(아래 '일정 파일'). 1.0 일정을 지우고 새로 가져오세요.
- 설정 파일(`notebook.env`)과 환경변수는 1.0과 같습니다. 1.0에서 쓰던 파일을 그대로 씁니다.

## 이미지 주소

두 곳에 같은 이미지를 올립니다. 업무망에서 받을 수 있는 쪽을 쓰세요.

| 레지스트리 | 주소 |
|---|---|
| GitHub (GHCR) | `ghcr.io/jameskwon07/manager-notebook:2.1.0` |
| Docker Hub | `docker.io/<DOCKERHUB_USER>/manager-notebook:2.1.0` |

- `<DOCKERHUB_USER>` 는 배포한 사람의 Docker Hub 이름으로 바꿔 읽으세요.
- **버전 태그(`2.1.0`)를 쓰세요.** `latest` 도 있지만, 언제 바뀔지 모릅니다.
- 1.0으로 되돌릴 때 쓰는 `1.0.0` 태그도 그대로 남아 있습니다.
- 회사가 레지스트리 미러(프록시)를 쓰면 주소 앞부분이 다를 수 있습니다. 예: `미러주소/jameskwon07/manager-notebook:2.1.0`. IT 부서에 물어보세요.
- linux/amd64, linux/arm64 를 모두 지원합니다.

## Windows 실행 파일로 쓰기 (Docker 없이)

Docker Desktop 을 쓸 수 없거나 설치가 번거로우면 실행 파일 하나로 씁니다. 화면과 기능은 Docker 판과 같습니다.

**1. 받기** (둘 중 하나)

- 인터넷 PC에서: [GitHub Releases](https://github.com/jameskwon07/manager-notebook/releases) 에서 `manager-notebook-2.1.0-windows-amd64.zip` 과 `SHA256SUMS.txt` 를 받아, 회사가 허용한 방법으로 옮깁니다.
- 레지스트리만 열린 망에서: Docker 가 있는 PC 한 대에서 묶음 이미지로 꺼냅니다. `-windows` 태그는 실행하는 이미지가 아니라 zip 을 담아 옮기는 묶음입니다.

```powershell
docker pull ghcr.io/jameskwon07/manager-notebook:2.1.0-windows
docker create --name nb-exe ghcr.io/jameskwon07/manager-notebook:2.1.0-windows x
docker cp nb-exe:/manager-notebook-2.1.0-windows-amd64.zip .
docker cp nb-exe:/SHA256SUMS.txt .
docker rm nb-exe
```

**2. 확인**: `Get-FileHash .\manager-notebook-2.1.0-windows-amd64.zip` 의 값이 `SHA256SUMS.txt` 와 같은지 봅니다.

**3. 실행**: 압축을 풀고 `manager-notebook.exe` 를 두 번 누릅니다. 검은 창이 뜨고 브라우저가 `http://localhost:8080` 으로 열립니다. 끄려면 창을 닫거나 `Ctrl+C` 를 누릅니다.

- 코드 서명이 없어 'Windows의 PC 보호' 창이 뜰 수 있습니다. **회사 PC 에서는 보안 담당자의 반입 승인을 먼저 받으세요.**
- 이 PC 안(127.0.0.1)에서만 열립니다. 같은 망의 다른 PC 에서는 열 수 없고, 방화벽 허용 창도 뜨지 않습니다.
- 한 번 더 실행하면 새로 띄우지 않고 켜져 있는 수첩을 브라우저로 엽니다. 8080 을 다른 프로그램이 쓰면 8081~8089 중 빈 포트를 씁니다.

**4. 기록 위치**: `%LOCALAPPDATA%\manager-notebook\notebook.db`. 실행 파일을 새 버전으로 바꿔도 남습니다. 다른 폴더를 쓰려면 `--data D:\nb` 를 붙입니다.

**5. 체험**: `run-demo.cmd` 를 두 번 누르면 `http://localhost:8090` 에서 예시 데이터로 열립니다. 창을 닫으면 사라집니다.

- **Docker 판에서 옮기기**: Docker 판 설정 탭 → 내보내기(.json) → 실행 파일 판 설정 탭 → 가져오기.
- **비밀번호·사내 AI**: Docker 판과 같은 환경변수를 씁니다. PowerShell 에서 `$env:LLM_BASE_URL="https://llm.example.internal/v1"` 처럼 정한 뒤, 같은 창에서 `.\manager-notebook.exe` 를 실행합니다.
- **옵션**: `--port 9000`(다른 포트), `--data 폴더`(다른 데이터 폴더), `--no-browser`(브라우저 안 엶), `--demo`(체험), `version`(버전).

## 준비물

- Windows 10/11 업무 PC
- Docker 를 쓸 수 없으면 위 'Windows 실행 파일로 쓰기' 로 갑니다. 아래는 Docker 판 준비물입니다.
- 아래 둘 중 하나
  - **Docker Desktop**
    - 직원 250명 이상이거나 연 매출 1천만 달러 이상인 회사는 **유료 구독**이 필요합니다.
    - 회사가 구독이 있는지 먼저 확인하세요.
  - **Rancher Desktop** (무료, 오픈소스)
    - 설치할 때 컨테이너 엔진을 **dockerd (moby)** 로 고르세요.
    - 설정 → Kubernetes → **Enable Kubernetes 를 끄세요.** 수첩에는 필요 없고, 메모리를 많이 씁니다.
    - 아래 `docker` 명령을 그대로 쓰도록 만들었지만, Rancher Desktop 에서는 아직 시험하지 못했습니다.
      처음 실행 4번의 확인 명령으로 꼭 확인하세요.
- 둘 다 WSL2 를 씁니다. 설치에 관리자 권한이나 IT 승인이 필요할 수 있습니다.

## 처음 실행 (Windows PowerShell)

1.0을 이미 쓰고 있다면 이 절 대신 아래 **'1.0에서 올리기'** 를 따르세요.

PowerShell 을 열고 한 줄씩 붙여 넣습니다.

**1. 이미지 받기** (둘 중 하나)

```powershell
docker pull ghcr.io/jameskwon07/manager-notebook:2.1.0
# 또는
docker pull docker.io/<DOCKERHUB_USER>/manager-notebook:2.1.0
```

`<DOCKERHUB_USER>` 는 배포한 사람의 Docker Hub 이름으로 바꿔 넣습니다.

이 뒤로는 받은 주소를 `$IMG` 에 넣고 씁니다. PowerShell 창을 새로 열면 이 줄을 다시 붙여 넣으세요.

```powershell
$IMG = "ghcr.io/jameskwon07/manager-notebook:2.1.0"
```

**2. 실행**

```powershell
docker run -d --name manager-notebook -p 127.0.0.1:8080:8080 -v manager-notebook:/data --restart unless-stopped $IMG
```

- 볼륨 `manager-notebook` 는 처음 실행할 때 저절로 만들어집니다. 모든 기록이 여기에 남습니다.
- 비밀번호나 사내 AI 를 쓰려면 아래 '설정 파일 쓰기' 를 먼저 보세요.

**3. 열기**

브라우저에서 <http://localhost:8080> 을 엽니다. 처음 열면 설정 탭에서 사람 표시와 쉬는 날을 확인하라는 안내가 나옵니다.

**4. 확인 (선택)**

`{"ok":true,"version":"2.1.0"}` 처럼 나오면 정상입니다.

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
docker run -d --name manager-notebook -p 127.0.0.1:8080:8080 -v manager-notebook:/data --restart unless-stopped --read-only --cap-drop ALL --security-opt no-new-privileges:true --log-opt max-size=10m --log-opt max-file=3 $IMG
```

## 1.0에서 올리기

1.0의 기록은 볼륨 안에 그대로 있습니다. 컨테이너만 2.0으로 바꾸고 **같은 볼륨**을 붙이면 됩니다.

**1. 1.0 화면에서 내보내기**

1.0 수첩의 설정 탭에서 '데이터 내보내기(.json)' 를 눌러 파일을 받아 둡니다. 문제가 생기면 이 파일로 되살립니다.

**2. 1.0이 쓰는 볼륨 이름 확인**

```powershell
docker inspect manager-notebook --format "{{range .Mounts}}{{.Name}} {{end}}"
```

나온 이름(1.0 README 대로 했다면 `manager-notebook`, 2.0 README 와 같은 이름)을 `$VOL` 에 넣습니다. 다른 이름이 나오면 그 이름을 씁니다.

```powershell
$VOL = "manager-notebook"
```

**3. 2.0 이미지 받기**

```powershell
$IMG = "ghcr.io/jameskwon07/manager-notebook:2.1.0"
docker pull $IMG
```

**4. 1.0 컨테이너 멈추고 지우기** (볼륨은 지우지 않습니다)

```powershell
docker stop manager-notebook
docker rm manager-notebook
```

**5. 같은 볼륨으로 2.0 실행**

1.0에서 `notebook.env` 를 썼다면 `--env-file` 을 그대로 붙입니다.

```powershell
docker run -d --name manager-notebook -p 127.0.0.1:8080:8080 -v "${VOL}:/data" --restart unless-stopped --env-file "$HOME\manager-notebook\notebook.env" $IMG
```

`notebook.env` 를 쓰지 않았다면 `--env-file ...` 부분을 빼고 실행합니다.

**6. 확인**

<http://localhost:8080> 을 열고 아래가 보이는지 봅니다.

- 제목이 `관리 수첩 2` 이고, 설정 탭의 버전이 `2.1.0`
- 수첩 탭: 1.0의 물음 메모·나중에 보낼 것
- 오늘 탭의 막힘 패널: 1.0의 열린 막힘(멈춤 상태도 그대로)
- 수첩 탭의 '1.0 기록': 1.0에서 저장한 금요일 요약

하나라도 안 보이면 더 쓰지 말고 아래 '되돌리기' 로 1.0에 돌아간 뒤 알려 주세요. 1번에서 받은 파일도 그대로 두세요.

**7. 일정 파일 바꾸기**

캘린더에서 1.0의 `[관리]` 일정(따로 만든 '관리 수첩' 캘린더면 그 캘린더)을 지우고, 2.0 설정 탭에서 일정 파일을 새로 받아 가져옵니다(아래 '일정 파일').

1.0 내보내기 파일을 2.0의 '가져오기' 에 넣어도 됩니다. 그때는 물음 메모·막힘·금요일·설정만 그 파일 내용으로 바뀌고, 2.0에서 만든 접수함·프로그램 같은 기록은 그대로 남습니다.

## 되돌리기 (2.0 → 1.0)

1.0 이미지는 `1.0.0` 태그로 계속 받을 수 있습니다. 같은 볼륨으로 1.0을 띄우면 됩니다.

먼저 1.0 이미지를 받아 두고, `$VOL` 을 다시 넣습니다. PowerShell 창을 새로 열었다면 `$VOL` 이 비어 있어 실행이 실패합니다. 볼륨 이름이 `manager-notebook` 이 아니면('1.0에서 올리기' 2번에서 확인한 이름) 따옴표 안을 바꿉니다.

```powershell
docker pull ghcr.io/jameskwon07/manager-notebook:1.0.0
$VOL = "manager-notebook"
docker rm -f manager-notebook
docker run -d --name manager-notebook -p 127.0.0.1:8080:8080 -v "${VOL}:/data" --restart unless-stopped --env-file "$HOME\manager-notebook\notebook.env" ghcr.io/jameskwon07/manager-notebook:1.0.0
```

`notebook.env` 를 쓰지 않았다면 `--env-file ...` 부분을 빼고 실행합니다.

- 2.0에서 만든 새 기록(접수함·프로그램·결정 로그·진행 지도·시간 일지·리스크 표 등)은 1.0 화면에 **보이지 않을 뿐 지워지지 않습니다.** 다시 2.0을 띄우면 그대로 보입니다.
- 2.0에서 저장한 금요일 기록은 1.0 화면에서 제대로 보이지 않을 수 있습니다.
- 1.0의 내보내기 파일에는 2.0 기록이 들어가지 않습니다. 되돌리기 전에 2.0 설정 탭에서 한 번 내보내 두세요.

## 설정 파일 쓰기 (비밀번호·사내 AI 를 쓸 때)

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

설정 파일을 쓰면 실행 명령에 `--env-file` 을 붙입니다.

```powershell
docker rm -f manager-notebook
docker run -d --name manager-notebook -p 127.0.0.1:8080:8080 -v manager-notebook:/data --restart unless-stopped --env-file "$HOME\manager-notebook\notebook.env" $IMG
```

**설정을 바꾼 뒤에는** 위처럼 컨테이너를 지우고 다시 만듭니다. 데이터는 볼륨에 있어서 그대로입니다.
`docker restart` 만으로는 바뀐 설정이 들어가지 않습니다.


## 비밀번호 켜기

같은 PC를 다른 사람도 쓴다면 켜 두세요.

1. `notebook.env` 에 한 줄을 넣습니다. **8자 이상**으로 적으세요(짧으면 시작 로그에 경고가 남습니다).
   ```
   NOTEBOOK_PASSWORD=여기에-긴-비밀번호
   ```
2. '설정 파일 쓰기' 의 마지막 명령대로 다시 만듭니다.
3. 이제 처음 열 때 로그인 화면이 나옵니다. 로그인은 12시간 유지됩니다.

- 끄려면 그 줄을 지우고 다시 만듭니다.
- 비밀번호를 잊으면 `notebook.env` 에서 새로 적고 다시 만듭니다.
- `notebook.env` 는 내 폴더에만 두고 공유하지 마세요.
- 비밀번호는 같은 PC의 다른 사람을 막는 용도입니다. 연결을 암호화하지 않으니, 다른 PC에서 열도록 바꾸지 마세요.

## 사내 AI 켜기 (선택)

기본은 꺼짐입니다. 꺼져 있어도 모든 기능이 틀(템플릿)로 동작합니다.
켜면 금요일 보고 초안 같은 곳에 '사내 AI로 다듬기' 버튼이 생깁니다. 다듬은 글이 마음에 들지 않으면 '되돌리기' 로 원래 초안을 돌려받습니다.

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
2. '설정 파일 쓰기' 의 마지막 명령대로 다시 만듭니다.
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
   docker run -d --name manager-notebook -p 127.0.0.1:8080:8080 -v manager-notebook:/data -v C:\certs\company-ca.pem:/certs/company-ca.pem:ro --restart unless-stopped --env-file "$HOME\manager-notebook\notebook.env" $IMG
   ```
4. `docker logs --tail 50 manager-notebook` 에서 `사내 인증서 1개 읽음` 을 확인합니다. 못 읽으면 `경고: SSL_CERT_FILE(...)에서 인증서를 못 읽었습니다` 가 보입니다.

`C:\` 경로를 `-v` 로 붙이는 것이 안 되면(Rancher Desktop 등), 인증서를 볼륨에 한 번 복사해 둡니다.

```powershell
docker volume create manager-notebook-certs
docker create --name nb-cert -v manager-notebook-certs:/certs busybox:1.37
docker cp C:\certs\company-ca.pem nb-cert:/certs/
docker rm -v nb-cert
```

그다음 실행 명령의 `-v C:\certs\company-ca.pem:/certs/company-ca.pem:ro` 대신 `-v manager-notebook-certs:/certs:ro` 를 붙입니다.

`LLM_INSECURE_SKIP_VERIFY=true` 는 인증서 확인을 끄는 마지막 수단입니다. 켜면 시작할 때 경고가 남습니다. 위 방법을 먼저 쓰세요.

## 업데이트 (2.0.x)

1. 먼저 설정 탭에서 내보내기를 합니다(아래 '백업과 복원').
2. 새 버전을 받습니다.
   ```powershell
   $IMG = "ghcr.io/jameskwon07/manager-notebook:2.0.1"
   docker pull $IMG
   ```
3. 컨테이너만 바꿉니다. 데이터는 볼륨에 남습니다.
   ```powershell
   docker rm -f manager-notebook
   docker run -d --name manager-notebook -p 127.0.0.1:8080:8080 -v manager-notebook:/data --restart unless-stopped $IMG
   ```
   (`notebook.env` 를 쓰면 `--env-file "$HOME\manager-notebook\notebook.env"` 를 붙입니다.)
4. 문제가 있으면 이전 버전 태그로 다시 만듭니다.

## 백업과 복원

**쉬운 방법: 설정 탭**

- '내보내기(.json)' 를 누르면 파일 하나로 받습니다. 2.0 파일에는 모든 탭의 기록과 설정이 들어 있습니다.
- 되살릴 때는 '가져오기' 를 씁니다. 두 번 눌러 확인합니다.
  - 2.0 내보내기 파일: 지금 데이터를 **모두 바꿉니다**.
  - 1.0 내보내기 파일: 물음 메모·막힘·금요일·설정만 바꾸고, 2.0에서 생긴 기록은 그대로 둡니다.

**볼륨 통째로**

이 이미지에는 셸이 없습니다(distroless). 그래서 공개 `busybox` 이미지로 압축합니다.
잠깐 멈추고 하면 파일이 깨지지 않습니다.

```powershell
New-Item -ItemType Directory -Force "$HOME\manager-notebook" | Out-Null
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

설정 탭의 '일정 파일 받기(.ics)' 를 누르면 시작일부터 12개월 동안의 관리 일정이 들어 있는 파일을 받습니다.

| 일정 | 언제 |
|---|---|
| 금요 읽기 | 매주 금요일 13:55. 쉬는 날인 금요일은 빠집니다 |
| 월간 리스크 15분 | 매달 첫 월요일 09:20. 그날이 쉬는 날이면 다음 근무일 |
| 사이클 시작 | 사이클마다 첫 월요일 09:20 (`[관리] 사이클 k 시작 — 자리 계산·지침 네 칸`) |
| 정리 주 시작 | 사이클마다 정리 주 월요일 09:20 |
| 3개월 점검 · 6개월 재검토 | 시작 주에서 3개월·6개월 뒤 그 주 월요일 09:20 |

- 모든 일정은 30분이고, 시작할 때 알림이 한 번 뜹니다. 제목은 모두 `[관리]` 로 시작합니다.
- 설명 안의 링크(`http://localhost:8080/#...`)를 누르면 수첩의 해당 탭이 열립니다.
- **'관리 수첩' 캘린더를 따로 만들어 넣기를 권합니다.** 그만둘 때 그 캘린더만 지우면 됩니다.
  - 클래식 Outlook: .ics 파일을 두 번 눌러 '새로 열기' 를 고르면 따로 생깁니다.
  - 새 Outlook·웹: '캘린더 추가 → 파일에서 업로드' 에서 새로 만든 캘린더를 고릅니다(메뉴 이름은 확인 필요).
  - 그룹웨어: 캘린더의 '가져오기' 를 씁니다.
- **1.0 일정 파일을 가져온 적이 있으면, 1.0 `[관리]` 일정을 먼저 모두 지웁니다.** 2.0 일정은 이름표(UID)가 달라서 덮어쓰지 않고 따로 생깁니다.
- **시작일·사이클·쉬는 날을 바꿨다면, 지난번 `[관리]` 일정을 모두 지운 뒤** 새 파일을 가져옵니다. 지우지 않으면 알림이 두 번씩 뜹니다.
- 가져온 뒤 한 번 확인하세요(기본 설정: 시작일 2026-10-19, 사이클 시작 2주차, 6주 + 정리 1주, 기본 쉬는 날).
  - 2026-10-23(금) 13:55 에 첫 금요 읽기가 있습니다.
  - 2026-11-02(월) 09:20 에 `[관리] 사이클 1 시작` 이 있습니다.
  - 2026-12-25(금)는 쉬는 날이라 금요 읽기가 없습니다.
  - 2027-03-02(화) 09:20 에 월간 리스크가 있습니다(03-01 삼일절에서 옮김).

## 지우기

```powershell
docker rm -f manager-notebook
docker volume rm manager-notebook
docker image rm $IMG
```

- `docker volume rm` 은 **모든 기록을 영구히 지웁니다.** 되돌릴 수 없습니다. 필요하면 먼저 내보내세요.
- `notebook.env` 와 백업 파일은 직접 지웁니다.
- 캘린더에 넣은 일정은 '관리 수첩' 캘린더를 지우거나, `[관리]` 로 시작하는 일정을 지우면 됩니다.

## 보안 성질

| 항목 | 내용 |
|---|---|
| 외부 연결 | 0개. 화면·글꼴·스크립트는 모두 이미지 안에 있습니다. 사내 AI 를 켰을 때 버튼을 누르는 경우만 그 주소로 나갑니다 |
| 네트워크 없이 동작 | `--network none` 으로 띄워도 모든 화면과 기능이 동작하는지 배포 전에 시험합니다(사내 AI 제외) |
| 실행 사용자 | nonroot(65532). 셸·패키지 관리자가 없는 distroless 이미지입니다 |
| 읽기 전용 | `--read-only --cap-drop ALL --security-opt no-new-privileges:true` 로 띄워도 동작합니다. 쓰기는 `/data` 볼륨에만 합니다 |
| 열리는 곳 | `-p 127.0.0.1:8080:8080` 으로 이 PC에서만 열립니다 |
| 브라우저 보호 | 모든 응답에 CSP(자기 주소만), `X-Frame-Options: DENY`, `nosniff`, `no-referrer` 를 붙입니다. 인라인 스크립트가 없습니다 |
| 로그 | 메모·요청 내용은 로그에 남기지 않습니다 |
| 이미지 내용 | 회사 이름, 사내 주소, 사람 이름, 키가 없습니다. 사내 값은 실행할 때 `notebook.env` 로만 넣습니다 |

## 이미지 검증

받은 이미지가 배포한 것과 같은지 확인할 수 있습니다. 인터넷이 되는 PC에서 해도 됩니다.

**digest 보기**

```powershell
docker buildx imagetools inspect ghcr.io/jameskwon07/manager-notebook:2.1.0
docker buildx imagetools inspect docker.io/<DOCKERHUB_USER>/manager-notebook:2.1.0
```

- 두 곳의 `Digest:` 값이 같아야 합니다.
- digest 로 받으면 태그가 바뀌어도 항상 같은 이미지를 받습니다.
  ```powershell
  docker pull ghcr.io/jameskwon07/manager-notebook@sha256:<digest>
  ```
- 이미 받은 이미지의 digest: `docker image inspect $IMG --format '{{json .RepoDigests}}'`

**SBOM(들어 있는 부품 목록) 보기**

```powershell
docker buildx imagetools inspect ghcr.io/jameskwon07/manager-notebook:2.1.0 --format '{{json .SBOM}}'
```

**빌드 기록(provenance) 보기**

```powershell
docker buildx imagetools inspect ghcr.io/jameskwon07/manager-notebook:2.1.0 --format '{{json .Provenance}}'
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
| 2.0으로 올렸는데 1.0 메모가 안 보임 | 다른 볼륨을 붙였을 가능성이 큽니다. `docker volume ls` 로 이름을 보고, '1.0에서 올리기' 2번에서 확인한 볼륨으로 다시 만듭니다. 새로 생긴 빈 볼륨은 지워도 됩니다 |
| 제목이 아직 `관리 수첩` (2 없음) | 브라우저가 옛 화면을 들고 있습니다. `Ctrl+F5` 로 새로 고칩니다. 그래도 같으면 `docker ps` 의 이미지 태그가 `2.1.0` 인지 봅니다 |
| `port is already allocated` | 8080 을 다른 프로그램(또는 지우지 않은 1.0 컨테이너)이 씁니다. `docker ps -a` 로 보고 지우거나, `-p 127.0.0.1:18080:8080` 으로 다시 실행합니다 |
| `Conflict. The container name "/manager-notebook" is already in use` | 같은 이름의 컨테이너가 남아 있습니다. `docker rm -f manager-notebook` 뒤에 다시 실행합니다(볼륨은 지워지지 않습니다) |
| `open ...notebook.env: no such file` | 설정 파일 경로가 틀렸습니다. `"$HOME\manager-notebook\notebook.env"` 가 있는지 봅니다. 쓰지 않는다면 `--env-file ...` 을 빼고 실행합니다 |
| `docker pull` 실패 | 회사 미러 주소나 프록시가 필요한지 IT 에 묻습니다. 다른 레지스트리 주소로도 해 봅니다 |
| 상태가 `unhealthy` | `docker logs --tail 50 manager-notebook` 과 `docker inspect --format "{{json .State.Health}}" manager-notebook` 을 봅니다 |
| `/data` 쓰기 권한 오류 | 폴더를 직접 붙이지 말고(`-v C:\...:/data`), 이름 있는 볼륨(`-v manager-notebook:/data`)을 쓰세요 |
| 가져오기가 거절됨 | 수첩에서 내보낸 `.json` 파일인지 봅니다(`"format":"manager-notebook-export"`). 거절되면 아무것도 바뀌지 않습니다 |
| 캘린더에 같은 일정이 두 번 | 1.0 일정이나 지난번 `[관리]` 일정을 지우지 않고 새 파일을 가져온 경우입니다. `[관리]` 일정을 모두 지우고 한 번만 가져옵니다 |
| 사내 AI 버튼이 안 보임 | 설정 탭의 사내 AI 상태, `LLM_BASE_URL` 철자, `--env-file` 을 붙여 다시 만들었는지 |
| 사내 AI 오류(502, 504) | `docker logs --tail 50 manager-notebook` 의 `llm connect error: <원인> (<호스트>)` 줄을 봅니다. `cert` 는 사내 인증서, `dns` 는 주소 철자, `proxy` 는 프록시, `refused` 는 주소·포트입니다. `llm upstream status 401` 처럼 숫자가 있으면 키·모델 이름·사내 AI 쪽 상태를 확인합니다. `3xx` 는 주소가 다른 곳으로 넘기는 것이니 `LLM_BASE_URL` 을 확인합니다(수첩은 따라가지 않습니다) |
| 사내 AI 에 프록시가 필요함 | `notebook.env` 에 `HTTPS_PROXY=...` 와 `NO_PROXY=...` 를 적습니다. 반대로 사내 AI 가 프록시를 타면 안 되면 `NO_PROXY=사내AI호스트` 를 적습니다 |
| `403` `host` 오류 | <http://localhost:8080> (포트를 바꿨으면 그 포트)으로 여세요. 다른 PC에서 여는 것은 권하지 않습니다 |

## 설정 값 전체

1.0과 같습니다.

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
go vet ./... && go test ./...                                      # Go 시험
node --test web/test/                                              # 화면 계산(core.js) 시험
docker build -t manager-notebook:dev --build-arg VERSION=2.1.0 .   # 빌드
EXPECTED_VERSION=2.1.0 scripts/smoke.sh manager-notebook:dev        # 연기 시험(--network none --read-only)
scripts/release.sh 2.1.0 <DOCKERHUB_USER>                           # 검사 후 두 레지스트리에 멀티아키 push
scripts/build-windows.sh 2.1.0 dist                                  # Windows 실행 파일 zip·SHA256SUMS (CI 는 Go 1.26 으로 빌드)
pwsh scripts/smoke-windows.ps1 -Exe .\manager-notebook.exe -Expected 2.1.0   # Windows 에서 실행 시험
```

- smoke 시험은 호스트 포트를 쓰지 않습니다(`--network none` 컨테이너 안에서 curl 로 요청). 이름이 `nb2-smoke-` 로 시작하는 컨테이너·볼륨을 만들고 끝나면 지웁니다.
  확인하는 것: health 버전, `/api/state` 컬렉션 11개, 2.0 컬렉션 8개의 만들기·고치기(null 은 키 삭제)·지우기, 없는 컬렉션 404, `/core.js`,
  내보내기 11개 → 가져오기 왕복, 1.0 모양 파일 가져오기(그 셋만 바뀜), 일정 파일(일정 30개, `ICS_EXPECTED_EVENTS`로 바꿈), 안내 4개, 보안 헤더, 사내 AI 꺼짐 404, 비밀번호·로그인, 볼륨 데이터 유지.
- **공개 확인.** `guide/*.md` 와 `web/content.json` 은 공개 이미지에 들어갑니다.
  첫 push 전에 팀 고유 정보(구체 시스템 이름, 사람 이름, 사내 이름)가 없는지 읽어 봅니다.
  `release.sh` 가 이 확인을 묻습니다(이미 확인했으면 `GUIDE_PUBLIC_OK=1`).
- `release.sh` 는 로그인하지 않습니다. 먼저 `docker login docker.io`, `docker login ghcr.io` 를 해 두세요.
- push 전에 smoke 시험과 trivy 검사(HIGH·CRITICAL 0건)를 통과해야 합니다. trivy 이미지는 digest 로 고정하고, Docker 소켓 대신 `docker save` 한 tar 를 검사합니다.
- 베이스 이미지(Dockerfile)와 GitHub 액션(release.yml)은 digest·커밋 SHA 로 고정했습니다. 올릴 때는 `.github/dependabot.yml` 의 제안을 봅니다.
- GitHub 에 `v2.1.0` 같은 태그를 올리면 `.github/workflows/release.yml` 이 같은 일을 합니다.
  Windows 실행 파일도 만들어 windows-latest 에서 실행 시험을 한 뒤, GitHub Release 에 zip 을 올리고 `<버전>-windows` 묶음 태그를 두 레지스트리에 올립니다.
  Docker Hub 에도 올리려면 저장소 Secrets 에 `DOCKERHUB_USERNAME`, `DOCKERHUB_TOKEN` 을 넣습니다.
- **GHCR 패키지는 처음 올리면 비공개입니다.** GitHub → Packages → manager-notebook → Package settings → Change visibility 에서 Public 으로 바꿔야 로그인 없이 받을 수 있습니다.
- Docker Hub 저장소도 Public 인지 확인합니다.

## 라이선스

MIT. [LICENSE](LICENSE) 를 보세요.
