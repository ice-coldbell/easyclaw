# PRD: EasyClaw v1 (Desktop-first)

## 문서 메타

- 버전: v1.0
- 상태: Draft (Execution-ready)
- 제품명: EasyClaw
- 기본 언어: 한국어 문서 및 UX 카피
- 연관 화면 문서: [screens.md](./screens.md)

## 1. Overview

### Problem

- AI Agent 트레이딩 전략을 빠르게 세팅하기 어렵고, 실행 상태/성과/리스크를 단일 콘솔에서 신뢰성 있게 확인하기 어렵다.
- 기존에는 직접 하나하나 구현해야하고, 관련된 플랫폼이 없거나 OnChain이 아닌 CEX다.

### Objective

- 회원 인증부터 Agent 실행까지의 경로를 최소화하고, 실시간 차트와 성과 화면에서 운영 의사결정을 빠르게 할 수 있게 한다.
- v1에서 `BTC Perpetual` 단일 마켓에 집중해 출시 속도와 운영 안정성을 우선 확보한다.

## 2. Goals / Non-Goals

### Goals

- G-01: TTV(Time-to-Value) 단축
- 가입 후 첫 Agent 실행(Paper 기준)까지 10분 이내 달성
- G-02: 실시간 모니터링 신뢰성
- 진입 시그널/체결 상태가 차트에 즉시 반영되고 E2E 지연이 2초 이내
- G-03: 운영 안전성
- Position limit, Daily loss limit, Kill switch로 실거래 사고를 예방
- G-04: 성과 비교 가능성
- Agent별/사용자 계정별 성과를 한눈에 비교하고 all-time 리더보드를 제공

### Non-Goals

- 멀티 자산/멀티 마켓/멀티 체인 지원
- 외부 DEX 다중 연동
- 모바일 네이티브 앱
- 전략 백테스트 엔진
- 팀 워크스페이스, 빌링/결제

## 3. Users & Use Cases

### Target Users

- Primary: Semi-pro Trader
- Secondary: Agent 성과를 비교/추적하려는 운영자 성향 사용자

### Key Use Cases

1. 신규 사용자 온보딩 (Agent-Centric)

- Landing에서 실시간 BTC 차트 + 활성 Agent 오더 히스토리로 플랫폼 가치 체감
- openclaw 에이전트에게 `npx clawhub@latest install easyclaw` 설치
- 에이전트의 Solana 지갑 준비 (신규 생성 또는 기존 지갑)
- 인간 지갑(Human Wallet) 서명으로 에이전트 지갑 소유권 서버 등록
- 전략 빌더에서 프리셋 선택 또는 커스텀 전략 설정
- 실행 → 에이전트가 WebSocket 실시간 데이터를 받아 자동 거래 시작

2. 운용 모니터링

- Trading Chart에서 에이전트의 진입/청산 포인트 확인 -> Agent Detail에서 상태 점검 -> 필요 시 Kill switch 실행

3. 성과 비교 + FOMO 유발

- Portfolio에서 내 에이전트 수익 확인 -> Leaderboard에서 다른 에이전트와 수익 비교 -> 상위 에이전트 추적 및 전략 재설정

## 4. Product Scope (v1)

### In Scope

- Home/Landing
- Onboarding + Agent Owner 바인딩 서명
- Wallet connect/sign (Solana ed25519)
- Trading Chart (BTC Perp only) + 실시간 진입 포인트
- Live Session Control
- Strategy Builder
- Agent Portfolio
- Agent Detail
- Trade History
- Risk Settings
- Agent Leaderboard
- Account/Connection Settings

### Out of Scope

- Backtest
- Multi-chain
- Multi-market
- Compliance 심화 모듈

## 5. Functional Requirements

### Must (v1 필수)

- FR-001: Landing에서 핵심 온보딩 카피(`지금 시장에, 내 에이전트를 바로 올리세요.`)와 실시간 BTC 차트에 플랫폼 활성 Agent들의 오더 히스토리를 동적으로 표시하여 가치 체감 및 시작 CTA를 제공한다.
- FR-002: Onboarding에서 지갑 메시지 서명 기반 `Agent-Owner(pubkey)` 바인딩 및 세션 발급을 지원한다.
- FR-003: Wallet challenge/verify-signature 흐름으로 Live 실행 step-up 권한을 검증한다.
- FR-004: Agent 생성/조회/상태 표시를 제공한다.
- FR-005: Agent별 리스크 프로필(max position, daily loss, kill switch)을 수정 가능해야 한다.
- FR-006: Agent 세션을 `paper|live`로 시작/중지할 수 있어야 한다.
- FR-007: BTC Perp 차트에 실시간 tick/signal/execution 오버레이를 표시한다.
- FR-008: Portfolio를 사용자 계정 단위 + Agent 단위로 조회 가능해야 한다.
- FR-009: Trade History에서 Agent/기간 기준 필터 조회를 지원한다.
- FR-010: Leaderboard는 all-time win_rate 기준으로 정렬하며 최소 20체결 조건을 적용한다.
- FR-011: win_rate 동률 시 `pnl_pct` -> `max_drawdown` 순서로 tie-break를 적용한다.
- FR-012: Position limit 초과 주문 차단, Daily loss limit 도달 시 신규 진입 차단, Kill switch 즉시 정지를 지원한다.
- FR-013: 세션 만료/nonce 만료, 서명 실패, 주문 실패, 데이터 지연에 대해 명확한 사용자 에러 메시지를 제공한다.
- FR-014: 시스템 상태(실시간 지연, DEX 연결 상태)를 노출한다.
- FR-023: Onboarding Flow에서 단계별 진행 안내와 함께 실시간 Agent 진입시점 프리뷰를 제공한다. (Landing의 차트와 동일 스타일)
- FR-021: Strategy Builder에서 미리 준비된 프리셋 선택 또는 커스텀 규칙(진입/청산), 파라미터(리스크 포함) 설정, 유효성 검증 후 저장을 지원한다.
- FR-022: Strategy Builder에서 저장한 전략을 Agent에 연결할 수 있어야 한다.
- FR-024: Onboarding에서 openclaw 에이전트에 `npx clawhub@latest install easyclaw` 설치 명령어를 안내하고 설치 완료를 감지한다.
- FR-025: Onboarding에서 에이전트의 Solana 지갑 주소(신규 생성 또는 기존 지갑)를 등록받는다.
- FR-026: 전략 실행 시 에이전트가 WebSocket으로 실시간 캔들 데이터를 수신하고, 전략에 따라 자동으로 거래를 실행한다. (v1: 캔들, 로드맵: 기술적 지표, 실시간 뉴스 피드)
- FR-027: Agent Leaderboard에서 다른 에이전트의 수익 현황을 실시간으로 확인할 수 있어 FOMO를 유발한다. (최신 수익률 변동, 순위 변동 애니메이션 포함)

### Should (v1 권장)

- FR-015: Live 전환 직전 위험도 요약(현재 노출, 손실 한도 잔여)을 표시한다.
- FR-016: Agent Detail에서 최근 성과 추세(일/주 단위)를 제공한다.
- FR-017: Account Settings에서 연결 상태 점검(세션 유효성, 지갑 바인딩 상태)을 제공한다.

### Could (후속 고려)

- FR-018: Leaderboard 고급 필터(거래수, 기간별 뷰)
- FR-019: Agent 성과 변화 알림(웹 알림/이메일)
- FR-020: 사용자 커스텀 위젯 배치

## 6. Non-Functional Requirements

- NFR-001 (Latency): 차트/진입포인트/체결 이벤트 E2E 지연 2초 이하 목표
- NFR-002 (Reliability): 데이터 지연/연결 불안정 시 `Degraded state` 전환 및 사용자 안내
- NFR-003 (Security): nonce 1회성/짧은 TTL, replay 방지, 서명 검증 실패 시 주문 차단, 감사 로그 보존
- NFR-004 (Observability): API/WS 지연, 오류율, 세션 상태 변경을 모니터링 가능해야 함
- NFR-005 (Platform): Web Desktop-first 최적화

## 7. Public APIs / Interfaces / Types

### REST API (v1)

- `POST /v1/auth/challenge` (intent: `owner_bind|session|live_stepup`)
- `POST /v1/auth/verify-signature`
- `POST /v1/auth/session/refresh`
- `GET /v1/agents/{agentId}/owner-binding`
- `POST /v1/agents/{agentId}/owner-binding/rebind`
- `GET /v1/agents`
- `POST /v1/agents`
- `PATCH /v1/agents/{agentId}/risk`
- `POST /v1/agents/{agentId}/sessions` (`paper|live`)
- `DELETE /v1/agents/{agentId}/sessions/{sessionId}`
- `POST /v1/safety/kill-switch`
- `GET /v1/strategy/templates`
- `POST /v1/strategies`
- `PATCH /v1/strategies/{strategyId}`
- `GET /v1/strategies/{strategyId}`
- `POST /v1/strategies/{strategyId}/publish`
- `GET /v1/portfolio`
- `GET /v1/leaderboard?metric=win_rate&period=all_time`
- `GET /v1/trades?agentId=&from=&to=`

### Realtime Channels

- `ws:chart.ticks.btc_perp` (캔들 데이터, 에이전트에게도 전달)
- `ws:agent.signals`
- `ws:agent.executions`
- `ws:portfolio.updates`
- `ws:leaderboard.updates` (순위 실시간 변동)
- `ws:system.status`
- (로드맵) `ws:indicators.btc_perp` (기술적 지표)
- (로드맵) `ws:news.feed` (실시간 뉴스 피드)

### Core Types

- `Agent`: id, name, strategy_type, status, created_at
- `SignalEvent`: agent_id, side, price, size, confidence, ts
- `ExecutionEvent`: order_id, fill_price, fill_qty, fee, tx_sig, ts
- `RiskProfile`: max_position_usd, daily_loss_limit_usd, kill_switch_enabled
- `StrategyDefinition`: id, name, template_id, entry_rules, exit_rules, risk_params, status, updated_at
- `PortfolioSnapshot`: user_equity, agent_equity, pnl_abs, pnl_pct, drawdown
- `LeaderboardEntry`: agent_id, win_rate, total_trades, pnl_pct, rank

## 8. Screen Mapping

- FR-001 -> SCR-01
- FR-002 -> SCR-02, SCR-03, SCR-04
- FR-003 -> SCR-04
- FR-004 -> SCR-07, SCR-08, SCR-13
- FR-005 -> SCR-10
- FR-006 -> SCR-06
- FR-007 -> SCR-05
- FR-008 -> SCR-07
- FR-009 -> SCR-09
- FR-010, FR-011 -> SCR-11
- FR-012 -> SCR-06, SCR-10
- FR-013, FR-014 -> SCR-03, SCR-04, SCR-05, SCR-06, SCR-12
- FR-017 -> SCR-12
- FR-021, FR-022 -> SCR-13
- FR-023 -> SCR-02
- FR-024 -> SCR-02
- FR-025 -> SCR-02, SCR-03
- FR-026 -> SCR-02, SCR-05, SCR-06
- FR-027 -> SCR-11

## 9. Success Metrics

- M-01 Activation: 첫 Agent 실행 전환율
- M-02 TTV: 가입 시점부터 첫 실행까지 소요 시간
- M-03 Monitoring Reliability: 2초 이하 이벤트 반영 비율
- M-04 Safety Effectiveness: 손실 한도 초과 방지율, Kill switch 성공률
- M-05 Retention: D7, D30 운영 지속률

## Onboarding Copy (v1)

- 대상 화면: `SCR-01 Home (Landing)` (메인 훅), `SCR-02 Onboarding Flow` (온보딩 진행 중)
- 헤드라인: `지금 시장에, 내 에이전트를 바로 올리세요.`
- 서브카피: `실시간 진입 시점을 보면서 전략을 연결하고, 몇 분 안에 에이전트가 자동으로 거래를 시작합니다.`

## 10. Test Cases / Acceptance Scenarios

1. Onboarding 완료 후 Agent-Owner 바인딩 서명 및 세션 발급이 정상 동작한다.
2. Wallet challenge-sign-verify 흐름이 성공/실패/nonce 재사용(replay) 케이스 모두 처리된다.
3. Trading Chart에 진입 포인트가 2초 이내 반영된다.
4. Paper 모드에서 주문 시뮬레이션과 포지션 반영이 일치한다.
5. Live 모드에서 주문 생성/체결 이벤트가 일관되게 기록된다.
6. Position limit 초과 주문이 차단된다.
7. Daily loss limit 도달 시 신규 진입이 차단된다.
8. Kill switch 실행 시 모든 활성 Agent가 정지된다.
9. Portfolio 값이 Trade History 합산과 일치한다.
10. Leaderboard가 `all-time win_rate` 기준으로 정렬된다.
11. 리더보드 동률 처리 규칙이 적용된다.
12. 세션 만료/서명 실패/DEX 지연 시 사용자 에러 메시지가 명확히 노출된다.
13. Strategy Builder에서 규칙/파라미터 유효성 검증 실패 시 저장이 차단되고 오류가 표시된다.
14. 저장된 Strategy를 Agent에 연결해 Paper/Live 세션 실행이 가능하다.
15. 첫 온보딩 화면에서 온보딩 문구와 실시간 Agent 진입시점 프리뷰가 표시되고, 데이터 지연 시 `Degraded` 안내와 재시도 상태가 노출된다.

## 11. Timeline (초안)

- Milestone 1: 온보딩 + Owner 바인딩 서명 + 세션 서명 흐름 완료
- Milestone 2: BTC 차트 + 실시간 시그널/체결 오버레이 완료
- Milestone 3: Strategy Builder + Agent 연동 완료
- Milestone 4: Portfolio/Detail/History/Leaderboard 완료
- Milestone 5: Safety controls + 운영 모니터링 + QA 완료

## 12. Risks & Dependencies

### Risks

- R-01: `No Disclaimer Screen` 요구로 인한 법적 고지 부족 리스크
- R-02: Custom DEX 온체인 지연/혼잡 시 체결 지연 체감 가능성
- R-03: Wallet signature UX 실패율 증가 시 활성화 저하
- R-04: 세션 만료/nonce 관리 실패(replay 취약) 시 인증 우회 또는 세션 중단 위험

### Dependencies

- D-01: EasyClaw Custom DEX (easyclaw-contract) RPC/Indexer 안정성
- D-02: Solana 지갑 연동 SDK 및 서명 검증 인프라
- D-03: 실시간 이벤트 파이프라인(WS/stream) 품질
- D-04: Agent 실행 엔진(Paper/Live 모드 전환) 일관성
- D-05: clawhub 패키지 레지스트리 안정성 (`npx clawhub@latest install easyclaw`)
- D-06: openclaw 에이전트 프레임워크와의 연동 호환성

## 13. Assumptions / Defaults

- 플랫폼: Web Desktop-first
- 마켓: BTC Perpetual only
- 실행: Paper + Live
- 인증: Agent 등록/소유권 검증은 Human Wallet message sign, 실거래 권한은 step-up Wallet signature
- 체인/서명: Solana (ed25519)
- DEX: 자체 Solana DEX (easyclaw-contract)
- 에이전트 프레임워크: openclaw (외부), clawhub를 통해 easyclaw 연동
- 에이전트 거래: 에이전트가 WebSocket 데이터를 수신하여 전략에 따라 자동 실행
- 리더보드 기준: all-time win_rate
- 리더보드 자격: 최소 20체결
- 동률 규칙: win_rate -> pnl_pct -> max_drawdown

## 14. Open Questions

- 현재 없음 (결정된 범위 기준으로 실행 가능)
