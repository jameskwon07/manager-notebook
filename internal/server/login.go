package server

// Built-in login page. It needs no JavaScript: the form posts to /api/login
// (form-urlencoded), which answers with a redirect. The front end may replace
// it by shipping web/login.html (and web/login.css).
const loginHTML = `<!doctype html>
<html lang="ko">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<meta name="referrer" content="no-referrer">
<title>관리 수첩 로그인</title>
<link rel="icon" href="/favicon.svg" type="image/svg+xml">
<link rel="stylesheet" href="/login.css">
</head>
<body>
<main class="box">
<h1>관리 수첩</h1>
<p class="hint">비밀번호를 적고 들어가세요.</p>
<!--MSG-->
<form method="post" action="/api/login">
<label for="pw">비밀번호</label>
<input id="pw" name="password" type="password" autocomplete="current-password" required autofocus>
<button type="submit">들어가기</button>
</form>
<p class="foot">이 화면은 이 PC 안에서만 동작합니다. 외부 연결은 없습니다.</p>
</main>
</body>
</html>
`

const loginCSS = `:root {
  --bg: #f6f5f2; --card: #ffffff; --text: #1d1d1b; --muted: #5f5e5a;
  --line: #d9d7d0; --accent: #2f5d50; --accent-text: #ffffff; --err: #a3352b;
}
@media (prefers-color-scheme: dark) {
  :root {
    --bg: #171716; --card: #222220; --text: #ecebe6; --muted: #a9a79f;
    --line: #3a3936; --accent: #7fb3a2; --accent-text: #10201b; --err: #f08b80;
  }
}
* { box-sizing: border-box; }
html, body { margin: 0; }
body {
  background: var(--bg); color: var(--text);
  font-family: "Malgun Gothic", "Apple SD Gothic Neo", system-ui, sans-serif;
  min-height: 100vh; display: flex; align-items: center; justify-content: center; padding: 16px;
}
.box {
  width: 100%; max-width: 360px; background: var(--card);
  border: 1px solid var(--line); border-radius: 12px; padding: 28px 24px;
}
h1 { font-size: 1.3rem; margin: 0 0 6px; }
.hint, .foot { color: var(--muted); font-size: .9rem; margin: 0 0 18px; }
.foot { margin: 18px 0 0; font-size: .8rem; }
.err { color: var(--err); font-size: .9rem; margin: 0 0 12px; }
label { display: block; font-size: .9rem; margin-bottom: 6px; }
input {
  width: 100%; font: inherit; padding: 10px 12px; border-radius: 8px;
  border: 1px solid var(--line); background: var(--bg); color: var(--text);
}
input:focus { outline: 2px solid var(--accent); outline-offset: 1px; }
button {
  margin-top: 14px; width: 100%; font: inherit; padding: 10px 12px; border: 0;
  border-radius: 8px; background: var(--accent); color: var(--accent-text); cursor: pointer;
}
`
