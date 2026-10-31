관리 수첩 {{VERSION}} - Windows 실행 파일

[실행]
1. manager-notebook.exe 를 두 번 누릅니다.
2. 검은 창이 뜨고 브라우저가 http://localhost:8080 으로 열립니다.
   브라우저가 안 열리면 그 주소를 직접 여세요.
3. 끄려면 검은 창을 닫거나 Ctrl+C 를 누릅니다.

회사 PC에서 쓰기 전에 보안 담당자의 반입 승인을 먼저 받으세요.
이 파일에는 코드 서명이 없습니다. 처음 실행할 때 "Windows의 PC 보호" 창이 뜨면,
회사 규정이 허용하는 경우에만 [추가 정보] → [실행]을 누릅니다.

[체험해 보기]
run-demo.cmd 를 두 번 누르면 예시 데이터로 열립니다(http://localhost:8090).
체험 모드의 기록은 메모리에만 있고, 창을 닫으면 사라집니다.

[기록은 어디에]
%LOCALAPPDATA%\manager-notebook\notebook.db
실행 파일을 새 버전으로 바꿔도 기록은 남습니다.
백업: 수첩의 설정 탭 → 내보내기(.json). 또는 수첩을 끈 뒤 위 폴더를 통째로 복사합니다.
Docker 판에서 옮겨 올 때: Docker 판 설정 탭에서 내보내기 → 이 판 설정 탭에서 가져오기.

[이미 켜져 있을 때]
한 번 더 실행하면 새로 띄우지 않고 켜져 있는 수첩을 브라우저로 엽니다.
8080 포트를 다른 프로그램이 쓰고 있으면 8081~8089 중 빈 포트를 씁니다.

[옵션] 명령 프롬프트나 PowerShell에서
  manager-notebook.exe --port 9000      다른 포트
  manager-notebook.exe --data D:\nb     다른 데이터 폴더
  manager-notebook.exe --no-browser     브라우저를 열지 않음
  manager-notebook.exe --demo           체험 모드
  manager-notebook.exe version          버전

[안전]
- 이 PC 안(127.0.0.1)에서만 열립니다. 같은 망의 다른 PC에서는 열 수 없습니다.
- 밖으로 나가는 연결이 없습니다. 사내 AI를 켰을 때만 그 주소로 갑니다.
- 받은 zip 파일의 SHA-256 값이 함께 배포된 SHA256SUMS.txt 와 같은지 확인하세요.
  PowerShell: Get-FileHash .\manager-notebook-{{VERSION}}-windows-amd64.zip

[더 보기]
https://github.com/jameskwon07/manager-notebook
MIT 라이선스 (LICENSE.txt)
