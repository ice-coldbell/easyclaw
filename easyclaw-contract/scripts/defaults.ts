export type RiskParams = {
  maxLeverage: number;
  imrBps: number;
  mmrBps: number;
  oiCap: bigint;
  skewCap: bigint;
  maxTradeNotional: bigint;
};

export type PricingParams = {
  baseSpreadBps: number;
  skewCoeffBps: number;
  maxFillDeviationBps: number;
  maxOracleStalenessSec: number;
  maxConfBps: number;
};

export type FundingParams = {
  intervalSec: number;
  fundingVelocityCapBpsPerDay: number;
  premiumClampBps: number;
};

export type FeeParams = {
  takerFeeBps: number;
  makerFeeBps: number;
};

export const BALANCED_COMMON = {
  risk: {
    maxLeverage: 20,
    imrBps: 500,
    mmrBps: 250
  },
  pricing: {
    baseSpreadBps: 5,
    maxFillDeviationBps: 150,
    maxOracleStalenessSec: 30,
    maxConfBps: 30
  },
  funding: {
    intervalSec: 3600,
    fundingVelocityCapBpsPerDay: 200,
    premiumClampBps: 100
  },
  fee: {
    takerFeeBps: 8,
    makerFeeBps: 2
  }
} as const;

export const MARKETS = [
  {
    marketId: 1,
    symbol: "BTC-PERP",
    risk: {
      ...BALANCED_COMMON.risk,
      oiCap: 5_000_000n * 1_000_000n,
      skewCap: 1_000_000n * 1_000_000n,
      maxTradeNotional: 250_000n * 1_000_000n
    },
    pricing: {
      ...BALANCED_COMMON.pricing,
      skewCoeffBps: 40
    }
  },
  {
    marketId: 2,
    symbol: "ETH-PERP",
    risk: {
      ...BALANCED_COMMON.risk,
      oiCap: 4_000_000n * 1_000_000n,
      skewCap: 800_000n * 1_000_000n,
      maxTradeNotional: 150_000n * 1_000_000n
    },
    pricing: {
      ...BALANCED_COMMON.pricing,
      skewCoeffBps: 55
    }
  },
  {
    marketId: 3,
    symbol: "SOL-PERP",
    risk: {
      ...BALANCED_COMMON.risk,
      oiCap: 2_000_000n * 1_000_000n,
      skewCap: 400_000n * 1_000_000n,
      maxTradeNotional: 75_000n * 1_000_000n
    },
    pricing: {
      ...BALANCED_COMMON.pricing,
      skewCoeffBps: 75
    }
  }
] as const;
