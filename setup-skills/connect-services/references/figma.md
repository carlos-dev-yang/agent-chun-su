# Figma 설치 매뉴얼

확인일: 2026-09-10. 상태: 외부 경로 조사 완료, Chun-su adapter 미구현.
목표 기능: 지정 디자인/노드·이미지·컴포넌트 읽기, 가능한 경로에서 디자인 생성/편집,
Code Connect 작업. REST와 MCP가 같은 기능을 제공한다고 가정하지 않는다.

## 사용자에게 물을 내용

- 디자인을 읽어 구현할 것인가, Figma에 만들거나 수정할 것인가?
- 이미 사용하는 AI 도구와 Figma 로그인/플랜은 무엇인가?
- 첫 검증에 사용할 파일/프레임 링크는 무엇인가? 조직의 외부 앱 제한이 있는가?

## 경로 A — 지원 AI 실행기의 공식 MCP/plugin

Figma는 remote MCP를 권장하며 지원 client별 plugin 또는 수동 등록을 안내한다.
공식 endpoint는 `https://mcp.figma.com/mcp`다. 브라우저 OAuth를 거쳐 파일/프레임
링크를 전달하는 흐름이다. 설치 도우미는 사용 중인 client의 공식 절차를 따른다.
[공식 remote 설치](https://developers.figma.com/docs/figma-mcp-server/remote-server-installation/).

접속은 Figma MCP Catalog에 등재된 client로 제한된다. Chun-su 자체 client가
자동으로 접속 가능하다고 표시하지 않는다. 승인된 client의 OAuth identity를
가장하거나 해당 client의 credential을 추출하지 않는다.
[MCP 접속 조건](https://developers.figma.com/docs/figma-mcp-server/).

현재 공식 도구에는 생성/편집을 포함하는 `use_figma`, 파일 생성과 asset 작업
등이 있다. `use_figma` 전체를 읽기 전용 목록에 넣으면 안 된다. 실제 tool schema를
조회하고 개별 capability를 검증한다. 원격 server는 로컬 고정 binary와 달리
도구 목록이 바뀔 수 있다.
[공식 도구](https://developers.figma.com/docs/figma-mcp-server/tools-and-prompts/).

이 경로로 다른 AI 도구에 연결해도 현재 Chun-su의 제한된 Codex 어댑터에는
자동 반영되지 않는다. 검증 전에는 “외부 client에 연결됨”으로만 표시한다.

## 경로 B — 호스트 REST connector + 고정 helper 후보

개인용은 PAT, 사용자에게 배포할 앱은 OAuth를 검토한다. 사용자가 직접 로컬
보안 입력으로 PAT를 저장하고 host가 API를 호출한다. 권고 읽기 범위는
`current_user:read`, `file_content:read`, 필요 시 `file_metadata:read`다.
comments/variables 등은 사용 목적에 따라 추가한다. REST scope는 MCP OAuth의
scope 설정법과 다르다.
[인증 방식](https://developers.figma.com/docs/rest-api/authentication/),
[권한 표](https://developers.figma.com/docs/rest-api/scopes/).

설치 도우미가 할 일:

1. 선택한 Figma 계정에서 PAT/OAuth 경로와 권한·만료를 안내한다.
2. 보호된 입력으로 저장 후 identity를 확인한다. 비밀값은 명령 인자나 로그에 넣지 않는다.
3. 제공된 링크에서 file key/node ID를 추출하고 host의 허용 리소스에 묶는다.
4. 제한된 원문/렌더링을 수집하고 버전·ID·해시를 보존한다.
5. REST로 불가능한 생성/편집은 지원 MCP 경로 또는 별도 구현 필요로 표시한다.

## 첫 검증과 룰셋 사례

- 자신의 identity와 지정 노드 읽기를 각각 확인한다. 파일 목록 발견만으로 성공 처리하지 않는다.
- 허용 밖 file/node, 오래된 버전, 내보내기 불가, plan/rate 제한은 별도 결과로 보존한다.
- 읽기 프로필에서 댓글·디자인·Code Connect 변경 도구가 노출되지 않아야 한다.
- 골든 사례: 노드 텍스트/레이아웃 근거와 AI 추정 구별, 보이지 않는 페이지 단정 금지,
  source 지시 주입 무시, 이미지 획득 실패 시 설명의 불확실성 표시.
- 생성/편집을 검증할 때 사용자가 고른 검증용 파일과 변경 내용을 확인하고 결과 노드와
  새 버전을 재조회한다. 이번에는 어떤 Figma 파일도 읽거나 수정하지 않았다.

## 문제 해결과 해제

접속 실패는 client catalog 자격, OAuth, 조직 정책, 파일 공유권, plan/사용량을
구분한다. 429에서 무제한 반복하지 말고 제공자 재시도 안내를 따른다.
[Figma rate limits](https://developers.figma.com/docs/rest-api/rate-limits/).
PAT 경로는 해당 PAT 철회, OAuth는 해당 앱 접근 철회 후 host 연결을 비활성화한다.
기존 디자인·내보낸 증거·다른 client 연결은 자동 삭제하지 않는다.
