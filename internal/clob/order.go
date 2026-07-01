// SPDX-License-Identifier: MIT

package clob

import (
	"crypto/rand"
	"fmt"
	"math"
	"math/big"

	"github.com/ethereum/go-ethereum/signer/core/apitypes"
)

// Side is a CLOB order side.
type Side string

const (
	Buy  Side = "BUY"
	Sell Side = "SELL"
)

// numeric returns the uint8 encoding of the side used in the signed EIP-712
// Order struct (0=BUY, 1=SELL).
func (s Side) numeric() (string, error) {
	switch s {
	case Buy:
		return "0", nil
	case Sell:
		return "1", nil
	default:
		return "", fmt.Errorf("invalid order side %q (expected BUY or SELL)", s)
	}
}

// Polymarket CTF Exchange V2 contract addresses on Polygon mainnet. Neg-risk
// markets settle through NegRiskExchangeAddress instead; the order book
// endpoint (GET /book) reports which applies to a given token via its
// neg_risk flag.
const (
	StandardExchangeAddress = "0xE111180000d2663C0091e4f400237545B87B996B"
	NegRiskExchangeAddress  = "0xe2222d279d744050d28e00520010520000310F59"
)

// zeroBytes32 fills the reserved metadata/builder fields when the caller
// (this server) has no builder attribution code to send: "0x" + 64 zero
// hex digits (32 zero bytes).
const zeroBytes32 = "0x0000000000000000000000000000000000000000000000000000000000000000"

// baseUnitScale is the fixed-point scale for both USDC-pegged collateral and
// Polymarket's ERC-1155 conditional (outcome) tokens: both use 6 decimals.
const baseUnitScale = 1_000_000

// orderType declares the CLOB V2 EIP-712 "Order" struct. Field order matters:
// it determines the canonical type string used in the struct hash. V2 dropped
// taker/expiration/nonce/feeRateBps (present in the historical V1 struct) and
// added timestamp/metadata/builder.
var orderType = apitypes.Types{
	"Order": []apitypes.Type{
		{Name: "salt", Type: "uint256"},
		{Name: "maker", Type: "address"},
		{Name: "signer", Type: "address"},
		{Name: "tokenId", Type: "uint256"},
		{Name: "makerAmount", Type: "uint256"},
		{Name: "takerAmount", Type: "uint256"},
		{Name: "side", Type: "uint8"},
		{Name: "signatureType", Type: "uint8"},
		{Name: "timestamp", Type: "uint256"},
		{Name: "metadata", Type: "bytes32"},
		{Name: "builder", Type: "bytes32"},
	},
}

// OrderArgs describes an order to build and sign in human units: Price is a
// probability in (0, 1) and Size is the outcome-token quantity.
type OrderArgs struct {
	TokenID string // ERC-1155 outcome token ID (decimal string)
	Price   float64
	Size    float64
	Side    Side
	// TickSize, when non-zero, rounds Price to the market's minimum price
	// increment (from GET /book) so the order isn't rejected for violating it.
	TickSize float64
	// Expiration is a wire-only field (not part of the signed struct): unix
	// seconds after which a GTD order expires. 0 = good-til-cancelled.
	Expiration int64
	// ExchangeAddress is the EIP-712 verifying contract: StandardExchangeAddress
	// unless the market is neg-risk (GET /book reports this), in which case it
	// must be NegRiskExchangeAddress.
	ExchangeAddress string
}

// SignedOrder is a fully-populated, signed CLOB V2 order. Fields match the
// wire shape POST /order expects for its nested "order" object; Expiration
// travels alongside the signed fields but is not itself signed.
type SignedOrder struct {
	Salt          string `json:"salt"`
	Maker         string `json:"maker"`
	Signer        string `json:"signer"`
	TokenID       string `json:"tokenId"`
	MakerAmount   string `json:"makerAmount"`
	TakerAmount   string `json:"takerAmount"`
	Side          Side   `json:"side"`
	SignatureType int    `json:"signatureType"`
	Timestamp     string `json:"timestamp"`
	Metadata      string `json:"metadata"`
	Builder       string `json:"builder"`
	Expiration    string `json:"expiration"`
	Signature     string `json:"signature"`
}

// BuildAndSign constructs and EIP-712 signs a CLOB V2 order. maker is the
// address funding the order (equal to the wallet's own address unless trading
// through a proxy or Gnosis Safe); signatureType selects the CTF Exchange
// signature scheme (0=EOA, 1=proxy, 2=Gnosis Safe) and must match how maker
// holds funds. nowMillis is the current unix time in milliseconds, used for
// the order's required "timestamp" field and as a distinguishing input to the
// random salt.
func BuildAndSign(w *Wallet, chainID int64, maker string, signatureType int, args OrderArgs, nowMillis int64) (*SignedOrder, error) {
	sideNum, err := args.Side.numeric()
	if err != nil {
		return nil, err
	}
	price := args.Price
	if args.TickSize > 0 {
		price = math.Round(price/args.TickSize) * args.TickSize
	}
	if price <= 0 || price >= 1 {
		return nil, fmt.Errorf("price must be between 0 and 1, got %v", price)
	}
	if args.Size <= 0 {
		return nil, fmt.Errorf("size must be positive, got %v", args.Size)
	}
	tokenID, ok := new(big.Int).SetString(args.TokenID, 10)
	if !ok {
		return nil, fmt.Errorf("invalid tokenId %q: not a decimal integer", args.TokenID)
	}

	tokenAmount := roundToAtomicUnits(new(big.Float).Mul(big.NewFloat(args.Size), big.NewFloat(baseUnitScale)))
	usdcAmount := roundToAtomicUnits(new(big.Float).Mul(
		new(big.Float).Mul(big.NewFloat(price), big.NewFloat(args.Size)),
		big.NewFloat(baseUnitScale),
	))

	var makerAmount, takerAmount *big.Int
	switch args.Side {
	case Buy:
		makerAmount, takerAmount = usdcAmount, tokenAmount
	case Sell:
		makerAmount, takerAmount = tokenAmount, usdcAmount
	}

	salt, err := randomSalt()
	if err != nil {
		return nil, err
	}

	exchange := args.ExchangeAddress
	if exchange == "" {
		exchange = StandardExchangeAddress
	}
	if maker == "" {
		maker = w.Address.Hex()
	}
	timestamp := fmt.Sprintf("%d", nowMillis)

	domain := apitypes.TypedDataDomain{
		Name:              "Polymarket CTF Exchange",
		Version:           "2",
		ChainId:           chainIDDomain(chainID),
		VerifyingContract: exchange,
	}
	message := apitypes.TypedDataMessage{
		"salt":          salt.String(),
		"maker":         maker,
		"signer":        w.Address.Hex(),
		"tokenId":       tokenID.String(),
		"makerAmount":   makerAmount.String(),
		"takerAmount":   takerAmount.String(),
		"side":          sideNum,
		"signatureType": fmt.Sprintf("%d", signatureType),
		"timestamp":     timestamp,
		"metadata":      zeroBytes32,
		"builder":       zeroBytes32,
	}

	sig, err := signTypedData(w, domain, "Order", orderType, message)
	if err != nil {
		return nil, fmt.Errorf("signing order: %w", err)
	}

	return &SignedOrder{
		Salt:          salt.String(),
		Maker:         maker,
		Signer:        w.Address.Hex(),
		TokenID:       tokenID.String(),
		MakerAmount:   makerAmount.String(),
		TakerAmount:   takerAmount.String(),
		Side:          args.Side,
		SignatureType: signatureType,
		Timestamp:     timestamp,
		Metadata:      zeroBytes32,
		Builder:       zeroBytes32,
		Expiration:    fmt.Sprintf("%d", args.Expiration),
		Signature:     sig,
	}, nil
}

// roundToAtomicUnits rounds a fixed-point amount to the nearest integer.
// big.Float.Int truncates toward zero, which would systematically round
// signed order amounts down by one atomic unit whenever floating-point
// representation error puts the true value a hair below its intended integer
// (e.g. 23331000 computed as 23330999.999999996) — a real, if tiny, price
// discrepancy in a signed, on-chain order. All amounts here are non-negative,
// so round-half-up is equivalent to round-to-nearest.
func roundToAtomicUnits(f *big.Float) *big.Int {
	rounded, _ := new(big.Float).Add(f, big.NewFloat(0.5)).Int(nil)
	return rounded
}

// randomSalt returns a cryptographically random uint256, used to make orders
// with otherwise-identical terms hash (and sign) uniquely.
func randomSalt() (*big.Int, error) {
	max := new(big.Int).Lsh(big.NewInt(1), 256)
	n, err := rand.Int(rand.Reader, max)
	if err != nil {
		return nil, fmt.Errorf("generating salt: %w", err)
	}
	return n, nil
}
