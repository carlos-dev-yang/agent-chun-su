# Google 공통 로그인 준비

Drive/Docs/Sheets와 Gmail 설정에서 중복 질문을 줄이기 위한 공통 안내다.
이미 승인된 client와 맞는 계정 연결이 있으면 재생성하지 않는다. **같은 계정**과
**같은 OAuth grant/권한**은 다르다. 이 문서는 실제 앱을 등록하지 않는다.

## 개인용 첫 설정

1. [Google Cloud 프로젝트 선택](https://console.cloud.google.com/projectselector2/home/dashboard)에서
   기존 프로젝트 또는 새 프로젝트를 선택한다. 생성은 해당 사용자의 권한으로 한다.
2. [API Library](https://console.cloud.google.com/apis/library)에서 선택 서비스의 API만
   활성화한다. Gmail은 Gmail API, Docs/Sheets는 해당 API와 필요한 Drive API를 고른다.
3. [Google Auth Platform](https://console.cloud.google.com/auth/overview)에서 앱 이름과
   지원 이메일·대상을 설정한다. 회사 Internal 사용 가능 여부는 조직 조건을 확인한다.
   External/Testing이면 본인 계정을 Test users에 넣는다.
4. [Credentials](https://console.cloud.google.com/apis/credentials)에서 OAuth client를
   만든다. 현재 Chun-su Gmail과 gws의 로컬 흐름은 **Desktop app** 경로다.
   원격 MCP/client의 redirect를 쓰면 해당 공식 가이드가 요구하는 client 유형으로
   별도 설정한다. Desktop client를 모든 remote client에 재사용하지 않는다.
5. 다운로드한 client JSON은 저장소 밖의 사용자 전용 보호 파일로 보관한다.
   설정 도우미에게는 위치만 전달한다. 동의 화면에서 실제 계정·권한을 확인한다.
6. 호스트가 callback과 token 저장을 처리하고 identity·실제 선택 리소스 호출을 확인한다.
   OAuth code나 전체 redirect URL을 AI 대화에 보내는 흐름은 채택하지 않는다.

공식 절차:
[client 만들기](https://developers.google.com/workspace/guides/create-credentials),
[설치형 OAuth](https://developers.google.com/identity/protocols/oauth2/native-app).

## 앱을 다른 사용자에게 배포할 때

각 사용자에게 Cloud Console 작업을 반복시키지 않는 로그인 버튼을 제공하려면
배포자 관리 OAuth 앱, 적절한 client 등록과 필요한 검토가 선행되어야 한다.
Gmail/Drive의 넓은 scope는 restricted 범주가 있으므로 데이터 전달/저장 구조에
맞는 제공자 검토를 준비한다. 내부용·개인용 trial의 성공으로 공개 배포 요건이
충족됐다고 하지 않는다.
[Gmail scope](https://developers.google.com/workspace/gmail/api/auth/scopes),
[Drive scope](https://developers.google.com/workspace/drive/api/guides/api-specific-auth).

초기 질문은 계정·기능·첫 리소스만 받고, client 생성 여부는 기존 설정을 확인한 뒤
필요한 경우에만 묻는다. Google 로그인 한 화면으로 묶을 수 있어도 기존 Gmail
읽기 credential을 무단 확장하지 않도록 grant 소유권과 철회 영향을 설계해야 한다.
