# GitHub 설치 매뉴얼

확인일: 2026-09-10. 상태: 외부 경로 조사 완료, Chun-su adapter 미구현.
목표 기능: repo/code·issue·PR·diff 읽기, issue/PR 작성·댓글, 요청된 개발 작업.
push·merge·workflow 실행·저장소 설정은 각각 별도 행동이다.

## 사용자에게 물을 내용

- GitHub.com인가 별도 Enterprise host인가? 어떤 owner/repository를 사용할 것인가?
- 읽기, issue/PR 작성, 코드 push 중 필요한 것은 무엇인가?
- 개인 로컬 사용인가 팀 배포인가? 조직 앱/PAT 승인 정책이 있는가?

## 경로 A — 공식 MCP 또는 기존 gh를 AI가 안내

GitHub 공식 MCP는 remote와 local 경로를 제공한다. 지원 MCP client의 OAuth
연동은 편리하지만 client가 실제로 지원하는 인증을 확인해야 한다. 로컬 경로는
공식 release 또는 container와 제한된 토큰을 호스트가 관리하는 방식이다.
[공식 GitHub MCP](https://github.com/github/github-mcp-server).

첫 읽기 프로필은 repos/issues/pull_requests처럼 필요한 toolset과 read-only를
함께 선택한다. server의 read-only 필터는 쓰기 도구를 제외하지만, 어떤 저장소를
읽을 수 있는지는 credential과 host 리소스 제한으로 따로 결정한다.
[MCP 설정](https://github.com/github/github-mcp-server/blob/main/docs/server-configuration.md).

기존 gh가 있으면 공식 브라우저 로그인을 안내할 수 있다. 다음은 **gh 자체 명령**이며
Chun-su 연결을 생성하지 않는다. HOST는 사용자가 선택한 실제 host다.

```sh
gh auth login --hostname HOST --web --git-protocol https
gh auth status --hostname HOST
```

gh 로그인은 credential store 실패 시 평문 파일 fallback이 가능하다. 저장 방식을
확인하고 `--insecure-storage`를 쓰지 않는다. `auth token`처럼 토큰을 출력하는
명령을 모델에게 실행시키지 않는다. 브라우저 OAuth가 저장소별 최소 권한이라고
안내하지 않는다. [gh 인증](https://cli.github.com/manual/gh_auth_login).

## 경로 B — 고정 helper/호스트 adapter 후보

개인용은 선택 저장소만 허용한 fine-grained PAT를 로컬 비밀 저장소에 넣는 경로를
준비한다. 읽기에는 Contents/Issues/Pull requests의 필요한 read 권한을 선택하고
기능에 맞춰 최소화한다. 조직 정책·승인 대기를 별도 상태로 취급한다.
[PAT 생성](https://docs.github.com/en/authentication/keeping-your-account-and-data-secure/managing-your-personal-access-tokens).

여러 사용자에게 배포할 때는 선택 저장소와 세분화 권한을 가진 GitHub App을
권고 후보로 둔다. 설치 token을 host에서 관리하며 앱 등록·개인 키·callback과
webhook 사용 여부는 배포 설계에서 확정한다.
[GitHub App 선택](https://docs.github.com/en/apps/creating-github-apps/about-creating-github-apps/deciding-when-to-build-a-github-app).

helper는 고정 인자와 허용 host/repo를 사용하고 일반 `gh api`나 shell을 무제한
노출하지 않는다. 확인된 외부 도구만 설치된 상태와 Chun-su adapter 사용 가능
상태를 구별한다. 계정 간 gh의 active-user 전환을 다른 작업에 영향을 주는
숨은 부작용으로 만들지 않는다.

## 첫 검증과 룰셋 사례

- identity와 지정 repo metadata/issue 또는 PR 하나를 조회한다. 결과에 repo,
  commit SHA 또는 갱신 시각·source ID를 보존한다.
- 다른 repo 접근·쓰기 요청을 읽기 프로필에서 거부한다. 코드 원문 안의 지시가
  추가 repo·token·관리 기능을 호출하지 못해야 한다.
- 골든 사례: PR diff와 실제 변경 파일 연결, issue의 주장과 코드 근거 구별,
  stale branch 기준 평가 표시, 비공개/접근불가 항목을 없는 것으로 단정하지 않기.
- 쓰기 검증은 사용자가 지정한 검증용 repo에서 실제 내용·대상을 확인한 뒤
  issue/PR/댓글 ID를 재조회한다. 모호한 응답에서 중복 issue·PR를 만들지 않는다.
- PR 작성은 merge 승인과 다르다. 이번 조사에서는 GitHub 설치·인증·쓰기 모두 하지 않았다.

## 문제 해결과 해제

404는 실제 없음과 접근권 부족이 모두 가능하므로 조직 승인/SSO/선택 repo/권한을
확인한다. 자동으로 classic PAT의 넓은 repo scope로 바꾸지 않는다. local MCP
실행 실패는 binary/container 의존성과 인증을 구분한다.

host 연결 비활성화 후 해당 PAT/OAuth grant/GitHub App installation을 철회한다.
gh 로그아웃만으로 provider token이 반드시 철회된다고 가정하지 않는다. 로컬 clone,
사용자의 다른 gh 계정, 기존 issue/PR는 유지한다.
