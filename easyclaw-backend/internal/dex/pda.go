package dex

import (
	"bytes"
	"encoding/binary"
	"fmt"

	"github.com/gagliardetto/solana-go"
)

func DeriveEngineConfigPDA(orderEngineProgramID solana.PublicKey) (solana.PublicKey, uint8, error) {
	return solana.FindProgramAddress([][]byte{[]byte("engine-config")}, orderEngineProgramID)
}

func DeriveEngineAuthorityPDA(orderEngineProgramID solana.PublicKey) (solana.PublicKey, uint8, error) {
	return solana.FindProgramAddress([][]byte{[]byte("engine-authority")}, orderEngineProgramID)
}

func DeriveFundingPDA(orderEngineProgramID solana.PublicKey, marketID uint64) (solana.PublicKey, uint8, error) {
	return solana.FindProgramAddress([][]byte{[]byte("funding"), u64LE(marketID)}, orderEngineProgramID)
}

func DeriveUserMarginPDA(orderEngineProgramID solana.PublicKey, user solana.PublicKey) (solana.PublicKey, uint8, error) {
	return solana.FindProgramAddress([][]byte{[]byte("user-margin"), user.Bytes()}, orderEngineProgramID)
}

func DeriveUserMarketPositionPDA(orderEngineProgramID solana.PublicKey, userMargin solana.PublicKey, marketID uint64) (solana.PublicKey, uint8, error) {
	return solana.FindProgramAddress([][]byte{[]byte("user-market-pos"), userMargin.Bytes(), u64LE(marketID)}, orderEngineProgramID)
}

func DeriveMarketPDA(marketRegistryProgramID solana.PublicKey, marketID uint64) (solana.PublicKey, uint8, error) {
	return solana.FindProgramAddress([][]byte{[]byte("market"), u64LE(marketID)}, marketRegistryProgramID)
}

func DeriveKeeperRebatePDA(lpVaultProgramID, pool, keeper solana.PublicKey) (solana.PublicKey, uint8, error) {
	return solana.FindProgramAddress([][]byte{[]byte("keeper-rebate"), pool.Bytes(), keeper.Bytes()}, lpVaultProgramID)
}

func U64LEToBytes(value uint64) []byte {
	return u64LE(value)
}

func SymbolString(symbol [16]uint8) string {
	index := bytes.IndexByte(symbol[:], 0)
	if index < 0 {
		index = len(symbol)
	}
	return string(symbol[:index])
}

func MustDeriveUserMarketPositionPDA(orderEngineProgramID solana.PublicKey, userMargin solana.PublicKey, marketID uint64) solana.PublicKey {
	pk, _, err := DeriveUserMarketPositionPDA(orderEngineProgramID, userMargin, marketID)
	if err != nil {
		panic(fmt.Errorf("derive user market position PDA: %w", err))
	}
	return pk
}

func u64LE(value uint64) []byte {
	buf := make([]byte, 8)
	binary.LittleEndian.PutUint64(buf, value)
	return buf
}
