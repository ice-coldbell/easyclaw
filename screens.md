# EasyClaw v1 화면 구성 (IA + Screen Spec)

## 1. 문서 목적
- 이 문서는 OpenClaw v1의 화면 정보구조(IA)와 각 화면의 구현 기준을 정의한다.
- 기능 요구사항은 [prd.md](./prd.md)의 `FR-*`를 기준으로 하며, 본 문서는 화면 단위 `SCR-*`를 관리한다.

## 2. IA / Navigation Map
1. Public 영역
- `SCR-01 Home (Landing)` -> `SCR-02 Onboarding Flow`
2. Onboarding 영역
- `SCR-02` -> `SCR-03 Agent Owner Binding` -> `SCR-04 Wallet Session Signature`
3. Authenticated App 영역
- 기본 진입: `SCR-05 Trading Chart`
- 보조: `SCR-06 Live Session Control`, `SCR-07 Agent Portfolio`, `SCR-08 Agent Detail`, `SCR-09 Trade History`, `SCR-10 Risk Settings`, `SCR-11 Agent Leaderboard`, `SCR-12 Account/Connection Settings`, `SCR-13 Strategy Builder`

## 3. 공통 상태 정의
- `STATE-EMPTY`: 아직 Agent/거래 데이터가 없는 상태
- `STATE-LOADING`: API 또는 WS 초기 로딩 상태
- `STATE-DEGRADED`: DEX 데이터 지연/부분 장애 상태
- `STATE-PERMISSION`: Live 실행 권한 없음 또는 미인증 상태
- `STATE-FAILURE`: 서명 실패, 세션/nonce 만료, 주문 실패 등 명시적 실패 상태

## 4. Screen Specs

### SCR-01 Home (Landing)
- Route: `/`
- 목적: 실시간 Agent 오더 히스토리가 표시된 BTC 차트로 플랫폼 가치를 즉시 체감시키고 온보딩 시작 유도
- 핵심 컴포넌트:
- Hero 섹션
  - 헤드라인: `지금 시장에, 내 에이전트를 바로 올리세요.`
  - 서브카피: `실시간 진입 시점을 보면서 전략을 연결하고, 몇 분 안에 에이전트가 자동으로 거래를 시작합니다.`
  - 시작 CTA
- 실시간 BTC 차트 (동적 메인 비주얼)
  - 플랫폼에서 활동 중인 Agent들의 오더 히스토리 오버레이 (롱/숏 색상 구분)
  - 실시간으로 신규 오더 마커가 찍히는 동적인 애니메이션
  - 최근 체결 이벤트 피드 (에이전트명, 방향, 가격, 시각)
  - 비로그인 방문자도 볼 수 있음 (소셜 프루프 + FOMO 유발)
- 핵심 기능 요약 (3-4 포인트: 에이전트 자동 거래, 전략 빌더, 리스크 제어, 성과 대시보드)
- 신뢰 요소 (자체 Solana DEX, BTC Perp 마켓, 에이전트 수 / 총 거래량 실시간 카운터)
- 상태:
- 적용: `STATE-LOADING` (차트 초기 로딩), `STATE-DEGRADED` (차트 데이터 지연 시 배너), `STATE-FAILURE` (서비스 장애 배너)
- 권한 조건: 비로그인/로그인 모두 접근 가능
- 주요 CTA:
- `시작하기` -> SCR-02
- `로그인` -> SCR-02
- 연관 요구사항: `FR-001`

### SCR-02 Onboarding Flow
- Route: `/onboarding`
- 목적: openclaw 에이전트가 EasyClaw에서 자동 거래를 시작하기까지의 5단계를 안내하는 마법사(Wizard) 화면
- 레이아웃: 좌측 단계 진행 패널 + 우측 실시간 BTC 차트 미니 프리뷰 (SCR-01과 동일 스타일)
- 단계 진행바: `에이전트 설치 → 지갑 준비 → 소유권 등록 → 전략 설정 → 실행`
- 핵심 컴포넌트 (단계별):

  - **Step 1: 에이전트 설치**
    - 안내 문구: "당신의 openclaw 에이전트에게 아래 명령어를 실행하도록 지시하세요."
    - 코드 블록 + 복사 버튼: `npx clawhub@latest install easyclaw`
    - 설치 완료 감지 폴링 (완료 시 Step 2로 자동 진행 또는 수동 완료 확인 버튼)

  - **Step 2: 에이전트 지갑 준비**
    - 안내 문구: "에이전트의 Solana 지갑 주소를 입력하세요."
    - 신규 지갑 생성 안내 또는 기존 지갑 주소 입력
    - 지갑 주소 유효성 검증 (Solana pubkey 형식)

  - **Step 3: 소유권 등록** -> SCR-03 (Agent Owner Binding)
    - 안내 문구: "당신의 지갑(Human Wallet)으로 에이전트 지갑의 소유권을 등록하세요."
    - 지갑 연결 + 서명으로 소유권 바인딩

  - **Step 4: 전략 설정** -> SCR-13 (Strategy Builder)
    - 안내 문구: "에이전트가 사용할 트레이딩 전략을 선택하거나 만드세요."
    - 미리 준비된 프리셋 선택 카드 (예: 모멘텀 추세추종, 평균회귀, 볼린저 브레이크아웃)
    - 또는 커스텀 전략 빌더로 이동

  - **Step 5: 실행**
    - 설정 요약 카드 (에이전트 지갑, 선택 전략, 리스크 파라미터)
    - Paper 모드로 시작 CTA (안전한 시뮬레이션 먼저)
    - 실행 후: 에이전트가 WebSocket으로 실시간 캔들 데이터를 수신하고 자동 거래 시작
    - 대시보드(SCR-05/SCR-07)로 이동 유도

- 우측 미니 프리뷰:
  - 실시간 BTC 차트 (Landing과 동일)
  - 활성 Agent 오더 마커 + 펄스 애니메이션
  - 최근 진입 이벤트 피드
- 상태:
- 적용: `STATE-LOADING`, `STATE-DEGRADED`, `STATE-FAILURE`
- Degraded 처리: 실시간 스트림 지연 시 마지막 수신 시각 및 재연결 상태 표시
- 권한 조건: 인증 사용자만 접근
- 주요 CTA:
- 각 단계 `다음` 버튼
- Step 5: `Paper로 시작` (-> SCR-05/SCR-06)
- 연관 요구사항: `FR-002`, `FR-023`, `FR-024`, `FR-025`, `FR-026`

### SCR-03 Agent Owner Binding
- Route: `/onboarding/agent-bind`
- 목적: Agent와 Owner Solana wallet pubkey를 메시지 서명으로 바인딩
- 핵심 컴포넌트:
- Agent 선택/확인 카드
- 서명 메시지 미리보기(intent, agent_id, owner_pubkey, nonce, expires_at)
- 바인딩 상태 카드(연결 상태, 마지막 검증 시간)
- 연결 성공 시 다음 단계 안내
- 상태:
- 적용: `STATE-LOADING`, `STATE-FAILURE`
- 실패 예시: nonce 만료, 서명 검증 실패, replay 감지
- 권한 조건: 인증 사용자만 접근
- 주요 CTA:
- `Owner 바인딩 서명`
- `다음 단계로` -> SCR-04
- 연관 요구사항: `FR-002`, `FR-013`

### SCR-04 Wallet Session Signature
- Route: `/onboarding/session-sign`
- 목적: Solana 지갑 연결 및 세션 서명 검증으로 앱 사용 권한 확보
- 핵심 컴포넌트:
- 지갑 선택/연결 모달
- Session challenge 문자열 표시
- 서명 요청/검증 결과 패널(세션 발급 결과)
- 상태:
- 적용: `STATE-LOADING`, `STATE-PERMISSION`, `STATE-FAILURE`
- 실패 예시: 서명 거부, 검증 실패, 세션 발급 실패
- 권한 조건: Agent Owner 바인딩 완료 사용자
- 주요 CTA:
- `지갑 연결`
- `세션 서명 요청`
- `대시보드로 이동` -> SCR-05
- 연관 요구사항: `FR-002`, `FR-003`, `FR-013`

### SCR-05 Trading Chart (BTC Perp)
- Route: `/app/chart`
- 목적: BTC Perp 실시간 차트와 Agent 진입/체결 이벤트 모니터링
- 핵심 컴포넌트:
- TradingView Lightweight Charts 캔들 차트
- 진입/청산 시그널 오버레이
- 체결 이벤트 타임라인
- 실시간 시스템 상태 배지(WS 지연, DEX 상태)
- 상태:
- 적용: `STATE-EMPTY`, `STATE-LOADING`, `STATE-DEGRADED`, `STATE-FAILURE`
- 권한 조건: 인증 사용자
- 주요 CTA:
- `세션 제어 열기` -> SCR-06
- `Agent 상세 보기` -> SCR-08
- 연관 요구사항: `FR-007`, `FR-014`, `FR-013`

### SCR-06 Live Session Control
- Route: `/app/sessions`
- 목적: Agent Paper/Live 실행 상태 제어 및 Kill switch 제공
- 핵심 컴포넌트:
- Agent별 세션 상태 테이블
- 모드 전환 토글(Paper/Live)
- 안전장치 패널(Position limit, Daily loss, Kill switch)
- 상태:
- 적용: `STATE-EMPTY`, `STATE-LOADING`, `STATE-PERMISSION`, `STATE-FAILURE`
- 권한 조건:
- Live 전환: Owner 바인딩 + 유효 세션 + live_stepup 서명 완료 필요
- 주요 CTA:
- `Paper 시작/중지`
- `Live 시작/중지`
- `Kill Switch 실행`
- 연관 요구사항: `FR-006`, `FR-012`, `FR-013`

### SCR-07 Agent Portfolio
- Route: `/app/portfolio`
- 목적: 사용자 계정/Agent 단위 자산과 성과를 종합 표시
- 핵심 컴포넌트:
- 계정 총자산 요약 카드
- Agent별 PnL/Drawdown 카드 리스트
- 기간 필터(기본 all-time)
- 상태:
- 적용: `STATE-EMPTY`, `STATE-LOADING`, `STATE-DEGRADED`
- 권한 조건: 인증 사용자
- 주요 CTA:
- `Agent 상세` -> SCR-08
- `거래 내역` -> SCR-09
- 연관 요구사항: `FR-004`, `FR-008`

### SCR-08 Agent Detail
- Route: `/app/agents/:agentId`
- 목적: 특정 Agent의 상태/성과/최근 시그널을 상세 확인
- 핵심 컴포넌트:
- Agent 프로필(전략 타입, 상태, 생성일)
- 최근 성과 추이(일/주)
- 최근 시그널/체결 리스트
- 상태:
- 적용: `STATE-EMPTY`, `STATE-LOADING`, `STATE-DEGRADED`
- 권한 조건: 인증 사용자
- 주요 CTA:
- `리스크 설정` -> SCR-10
- `세션 제어` -> SCR-06
- 연관 요구사항: `FR-004`, `FR-016`

### SCR-09 Trade History
- Route: `/app/trades`
- 목적: Agent/기간별 거래 내역 조회 및 분석
- 핵심 컴포넌트:
- 필터 바(agentId, from, to)
- 거래 테이블(진입/청산, 수량, 수수료, tx_sig)
- 정합성 요약(Portfolio와 합산 비교)
- 상태:
- 적용: `STATE-EMPTY`, `STATE-LOADING`, `STATE-DEGRADED`
- 권한 조건: 인증 사용자
- 주요 CTA:
- `필터 적용`
- `Agent 상세` -> SCR-08
- 연관 요구사항: `FR-009`

### SCR-10 Risk Settings
- Route: `/app/risk`
- 목적: Agent별 리스크 프로필과 안전장치 한도 관리
- 핵심 컴포넌트:
- max_position_usd 입력
- daily_loss_limit_usd 입력
- kill_switch_enabled 토글
- 저장 전 변경점 확인 패널
- 상태:
- 적용: `STATE-EMPTY`, `STATE-LOADING`, `STATE-FAILURE`
- 권한 조건: 인증 사용자
- 주요 CTA:
- `리스크 설정 저장`
- `Kill Switch 실행`
- 연관 요구사항: `FR-005`, `FR-012`

### SCR-11 Agent Leaderboard
- Route: `/app/leaderboard`
- 목적: Agent 수익 순위를 실시간으로 보여주며 경쟁심과 FOMO를 유발, 성과 비교를 통한 전략 개선 동기 부여
- 핵심 컴포넌트:
- 상단 요약 배너: 현재 1위 에이전트의 수익률 + "지금 이 순간도 수익 중" 실시간 표시
- 리더보드 테이블 (실시간 업데이트)
  - rank (순위 변동 화살표 애니메이션), agent_name, win_rate, pnl_pct, total_trades, 최근 수익 변동 스파크라인
- 내 에이전트 순위 하이라이트 (로그인 시 내 에이전트 행 강조)
- 최근 순위 변동 알림 피드 (우측 사이드: "Agent X가 방금 3위로 올라섰습니다")
- 최소 체결 수 필터(20건 기본)
- tie-break 안내(win_rate -> pnl_pct -> max_drawdown)
- 상태:
- 적용: `STATE-EMPTY`, `STATE-LOADING`, `STATE-DEGRADED`
- 권한 조건: 인증 사용자
- 주요 CTA:
- `Agent 상세 보기` -> SCR-08
- `전략 수정하기` -> SCR-13 (내 에이전트 순위가 낮을 때 유도)
- 연관 요구사항: `FR-010`, `FR-011`, `FR-027`

### SCR-12 Account / Connection Settings
- Route: `/app/settings/connections`
- 목적: Owner 바인딩/지갑 연결/세션 상태 확인 및 재서명 관리
- 핵심 컴포넌트:
- Agent-Owner 바인딩 상태
- 세션 상태(유효/만료/갱신)
- Wallet 바인딩 상태
- 시스템 상태(DEX 연결성, WS health)
- 상태:
- 적용: `STATE-LOADING`, `STATE-PERMISSION`, `STATE-FAILURE`
- 권한 조건: 인증 사용자
- 주요 CTA:
- `Owner 재바인딩`
- `세션 재서명`
- 연관 요구사항: `FR-012`, `FR-013`, `FR-014`, `FR-017`

### SCR-13 Strategy Builder
- Route: `/app/strategy-builder`
- 목적: 사용자 정의 전략을 생성/수정하고 Agent에 연결 가능한 상태로 저장
- 핵심 컴포넌트:
- 전략 템플릿 선택기
- 규칙 빌더(진입/청산 조건 블록)
- 리스크 파라미터 패널(포지션 크기, 손실 한도 기본값 등)
- 유효성 검사 결과 패널(필수값 누락, 상충 규칙, 범위 오류)
- 저장/게시 상태 배지(draft/published)
- 상태:
- 적용: `STATE-EMPTY`, `STATE-LOADING`, `STATE-FAILURE`
- 실패 예시: 유효성 검증 실패, 저장 실패, 게시 실패
- 권한 조건: 인증 사용자
- 주요 CTA:
- `전략 저장`
- `전략 게시`
- `Agent에 연결`
- 연관 요구사항: `FR-004`, `FR-021`, `FR-022`

## 5. 화면-요구사항 매트릭스
- SCR-01 -> FR-001
- SCR-02 -> FR-002, FR-023, FR-024, FR-025, FR-026
- SCR-03 -> FR-002, FR-013, FR-025
- SCR-04 -> FR-002, FR-003, FR-013
- SCR-05 -> FR-007, FR-013, FR-014, FR-026
- SCR-06 -> FR-006, FR-012, FR-013, FR-026
- SCR-07 -> FR-004, FR-008
- SCR-08 -> FR-004, FR-016
- SCR-09 -> FR-009
- SCR-10 -> FR-005, FR-012
- SCR-11 -> FR-010, FR-011, FR-027
- SCR-12 -> FR-012, FR-013, FR-014, FR-017
- SCR-13 -> FR-004, FR-021, FR-022
