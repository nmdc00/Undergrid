package sandbox

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math/big"
	"strings"
)

type usdcHeader struct {
	Number    string `json:"number"`
	Hash      string `json:"hash"`
	Timestamp string `json:"timestamp"`
}
type usdcLog struct {
	Address     string   `json:"address"`
	Topics      []string `json:"topics"`
	Data        string   `json:"data"`
	Transaction string   `json:"transactionHash"`
	BlockHash   string   `json:"blockHash"`
	Block       string   `json:"blockNumber"`
	Index       string   `json:"logIndex"`
	Removed     bool     `json:"removed"`
}
type usdcReceipt struct {
	Transaction string    `json:"transactionHash"`
	BlockHash   string    `json:"blockHash"`
	Block       string    `json:"blockNumber"`
	Status      string    `json:"status"`
	Logs        []usdcLog `json:"logs"`
}
type usdcTransaction struct {
	Hash      string `json:"hash"`
	To        string `json:"to"`
	Input     string `json:"input"`
	BlockHash string `json:"blockHash"`
	Block     string `json:"blockNumber"`
	Chain     string `json:"chainId"`
}
type usdcObservation struct {
	Transaction, BlockHash, Block, Finality, TransferIndex, AuthorizationIndex string
	Evidence                                                                   json.RawMessage
}

func usdcSelector(signature string) string {
	return "0x" + hex.EncodeToString(walletKeccak256([]byte(signature))[:4])
}
func usdcTopic(signature string) string {
	return "0x" + hex.EncodeToString(walletKeccak256([]byte(signature)))
}
func usdcAddressWord(address string) string {
	return "0x" + strings.Repeat("0", 24) + strings.TrimPrefix(address, "0x")
}
func (s *USDCService) checkChain(ctx context.Context) error {
	var chain string
	if err := s.rpc(ctx, "eth_chainId", []any{}, &chain); err != nil {
		return err
	}
	n, err := chainQuantity(chain)
	if err != nil || n.Int64() != USDCTestChain || !n.IsInt64() {
		return ErrConflict
	}
	return nil
}
func (s *USDCService) call(ctx context.Context, data string) (string, error) {
	var result string
	err := s.rpc(ctx, "eth_call", []any{map[string]any{"to": USDCTestToken, "data": data}, "latest"}, &result)
	return result, err
}
func decodeABIString(raw string) (string, error) {
	if !strings.HasPrefix(raw, "0x") || len(raw) > 1026 {
		return "", ErrInvalid
	}
	body, err := hex.DecodeString(raw[2:])
	if err != nil || len(body) < 64 || new(big.Int).SetBytes(body[:32]).Cmp(big.NewInt(32)) != 0 {
		return "", ErrInvalid
	}
	size := new(big.Int).SetBytes(body[32:64])
	if !size.IsInt64() || size.Int64() < 1 || size.Int64() > 256 || int64(len(body)) < 64+size.Int64() {
		return "", ErrInvalid
	}
	return string(body[64 : 64+size.Int64()]), nil
}
func (s *USDCService) header(ctx context.Context, tag string) (*usdcHeader, error) {
	var header *usdcHeader
	if err := s.rpc(ctx, "eth_getBlockByNumber", []any{tag, false}, &header); err != nil {
		return nil, err
	}
	if header == nil || !chainHash(header.Hash) {
		return nil, ErrInvalid
	}
	if _, err := chainQuantity(header.Number); err != nil {
		return nil, err
	}
	if _, err := chainQuantity(header.Timestamp); err != nil {
		return nil, err
	}
	return header, nil
}
func (s *USDCService) VerifyConfiguration(ctx context.Context) error {
	if err := s.checkChain(ctx); err != nil {
		return err
	}
	var code string
	if err := s.rpc(ctx, "eth_getCode", []any{USDCTestToken, "latest"}, &code); err != nil {
		return err
	}
	if !strings.HasPrefix(code, "0x") || len(code) < 4 || !validLowerHex(strings.ToLower(code[2:])) {
		return ErrConflict
	}
	raw, err := s.call(ctx, usdcSelector("decimals()"))
	if err != nil {
		return err
	}
	decimals, ok := new(big.Int).SetString(strings.TrimPrefix(raw, "0x"), 16)
	if !ok || decimals.Cmp(big.NewInt(6)) != 0 {
		return ErrConflict
	}
	for _, property := range []struct{ name, want string }{{"name()", s.config.TokenName}, {"version()", s.config.TokenVersion}} {
		raw, err = s.call(ctx, usdcSelector(property.name))
		if err != nil {
			return err
		}
		got, err := decodeABIString(raw)
		if err != nil || got != property.want {
			return ErrConflict
		}
	}
	for _, tag := range []string{"safe", "finalized"} {
		if _, err = s.header(ctx, tag); err != nil {
			return err
		}
	}
	var supported struct {
		Kinds []struct {
			Version int    `json:"x402Version"`
			Scheme  string `json:"scheme"`
			Network string `json:"network"`
		} `json:"kinds"`
	}
	if err = s.request(ctx, "GET", strings.TrimRight(s.config.FacilitatorURL, "/")+"/supported", nil, &supported); err != nil {
		return err
	}
	for _, kind := range supported.Kinds {
		if kind.Version == 2 && kind.Scheme == "exact" && kind.Network == "eip155:84532" {
			s.verified.Store(true)
			return nil
		}
	}
	return ErrConflict
}
func usdcTransferCalldata(record WalletPaymentAuthorization) (string, error) {
	c := record.WalletPaymentCommand
	from, err := abiAddress(c.Authorization.From)
	if err != nil {
		return "", err
	}
	to, err := abiAddress(c.Authorization.To)
	if err != nil {
		return "", err
	}
	value, err := abiUint256(c.Authorization.Value)
	if err != nil {
		return "", err
	}
	after, err := abiUint256(c.Authorization.ValidAfter)
	if err != nil {
		return "", err
	}
	before, err := abiUint256(c.Authorization.ValidBefore)
	if err != nil {
		return "", err
	}
	nonce, err := hex.DecodeString(strings.TrimPrefix(c.Authorization.Nonce, "0x"))
	if err != nil || len(nonce) != 32 {
		return "", ErrInvalid
	}
	signature, err := hex.DecodeString(strings.TrimPrefix(c.Signature, "0x"))
	if err != nil || len(signature) != 65 {
		return "", ErrInvalid
	}
	v := int64(signature[64])
	if v < 27 {
		v += 27
	}
	if v != 27 && v != 28 {
		return "", ErrInvalid
	}
	parity := big.NewInt(v).FillBytes(make([]byte, 32))
	words := bytes.Join([][]byte{from, to, value, after, before, nonce, parity, signature[:32], signature[32:64]}, nil)
	return usdcSelector("transferWithAuthorization(address,address,uint256,uint256,uint256,bytes32,uint8,bytes32,bytes32)") + hex.EncodeToString(words), nil
}
func (s *USDCService) verifyReceipt(ctx context.Context, p Payment, transaction string, start *big.Int) (*usdcObservation, error) {
	if !chainHash(transaction) || p.walletAuthorization == nil {
		return nil, ErrInvalid
	}
	var receipt *usdcReceipt
	if err := s.rpc(ctx, "eth_getTransactionReceipt", []any{transaction}, &receipt); err != nil {
		return nil, err
	}
	if receipt == nil {
		return nil, nil
	}
	number, err := chainQuantity(receipt.Block)
	if err != nil || receipt.Transaction != transaction || !chainHash(receipt.BlockHash) || receipt.Status != "0x1" || (start != nil && number.Cmp(start) <= 0) {
		return nil, ErrConflict
	}
	canonical, err := s.header(ctx, receipt.Block)
	if err != nil {
		return nil, err
	}
	if canonical.Hash != receipt.BlockHash || canonical.Number != receipt.Block {
		return nil, ErrConflict
	}
	timestamp, _ := chainQuantity(canonical.Timestamp)
	after, _ := new(big.Int).SetString(p.walletAuthorization.Authorization.ValidAfter, 10)
	before, _ := new(big.Int).SetString(p.walletAuthorization.Authorization.ValidBefore, 10)
	if after == nil || before == nil || timestamp.Cmp(after) <= 0 || timestamp.Cmp(before) >= 0 {
		return nil, ErrConflict
	}
	var tx *usdcTransaction
	if err = s.rpc(ctx, "eth_getTransactionByHash", []any{transaction}, &tx); err != nil {
		return nil, err
	}
	expected, err := usdcTransferCalldata(*p.walletAuthorization)
	if err != nil {
		return nil, err
	}
	if tx == nil || tx.Hash != transaction || strings.ToLower(tx.To) != USDCTestToken || strings.ToLower(tx.Input) != expected || tx.BlockHash != receipt.BlockHash || tx.Block != receipt.Block {
		return nil, ErrConflict
	}
	chain, err := chainQuantity(tx.Chain)
	if err != nil || !chain.IsInt64() || chain.Int64() != USDCTestChain {
		return nil, ErrConflict
	}
	proof := &usdcObservation{Transaction: transaction, BlockHash: receipt.BlockHash, Block: number.String(), Finality: "provisional"}
	auth, transfer := 0, 0
	authorization := p.walletAuthorization.Authorization
	for _, log := range receipt.Logs {
		if strings.ToLower(log.Address) != USDCTestToken {
			continue
		}
		if log.Removed || log.Transaction != transaction || log.BlockHash != receipt.BlockHash || log.Block != receipt.Block {
			return nil, ErrConflict
		}
		index, err := chainQuantity(log.Index)
		if err != nil {
			return nil, err
		}
		if len(log.Topics) == 3 && log.Topics[0] == usdcTopic("AuthorizationUsed(address,bytes32)") && log.Topics[1] == usdcAddressWord(authorization.From) && log.Topics[2] == authorization.Nonce && log.Data == "0x" {
			auth++
			proof.AuthorizationIndex = index.String()
		}
		if len(log.Topics) == 3 && log.Topics[0] == usdcTopic("Transfer(address,address,uint256)") && log.Topics[1] == usdcAddressWord(authorization.From) && log.Topics[2] == usdcAddressWord(authorization.To) {
			data, err := hex.DecodeString(strings.TrimPrefix(log.Data, "0x"))
			if err != nil || len(data) != 32 {
				return nil, ErrConflict
			}
			if new(big.Int).SetBytes(data).String() == authorization.Value {
				transfer++
				proof.TransferIndex = index.String()
			}
		}
	}
	if auth != 1 || transfer != 1 || proof.TransferIndex == proof.AuthorizationIndex {
		return nil, ErrConflict
	}
	safe, err := s.header(ctx, "safe")
	if err != nil {
		return nil, err
	}
	safeNumber, _ := chainQuantity(safe.Number)
	if safeNumber.Cmp(number) >= 0 {
		proof.Finality = "safe"
	}
	finalized, err := s.header(ctx, "finalized")
	if err != nil {
		return nil, err
	}
	finalizedNumber, _ := chainQuantity(finalized.Number)
	if finalizedNumber.Cmp(safeNumber) > 0 {
		return nil, ErrConflict
	}
	if finalizedNumber.Cmp(number) >= 0 {
		proof.Finality = "finalized"
	}
	// Recheck the inclusion block after finality lookups; an inconsistent/forking
	// provider snapshot never becomes a finalized fact by combining two branches.
	again, err := s.header(ctx, receipt.Block)
	if err != nil {
		return nil, err
	}
	if again.Hash != receipt.BlockHash {
		return nil, ErrConflict
	}
	proof.Evidence = encode(map[string]any{"chain_id": USDCTestChain, "token": USDCTestToken, "payer": authorization.From, "payee": authorization.To, "nonce": authorization.Nonce, "gross_atomic": authorization.Value, "net_atomic": p.Quote.Amount, "fee_atomic": p.Quote.Fee, "transaction_hash": transaction, "block_hash": proof.BlockHash, "block_number": proof.Block, "authorization_log_index": proof.AuthorizationIndex, "transfer_log_index": proof.TransferIndex, "calldata_hash": hash([]byte(expected)), "authorization_hash": p.WalletAuthorizationHash, "finalized_head": finalized.Number, "safe_head": safe.Number, "designation": "TEST"})
	return proof, nil
}
func (s *USDCService) nonceUsed(ctx context.Context, p Payment) (bool, error) {
	if p.walletAuthorization == nil {
		return false, ErrUnauthorized
	}
	data := usdcSelector("authorizationState(address,bytes32)") + strings.TrimPrefix(usdcAddressWord(p.walletAuthorization.Authorization.From), "0x") + strings.TrimPrefix(p.walletAuthorization.Authorization.Nonce, "0x")
	raw, err := s.call(ctx, data)
	if err != nil {
		return false, err
	}
	body, err := hex.DecodeString(strings.TrimPrefix(raw, "0x"))
	if err != nil || len(body) != 32 {
		return false, ErrInvalid
	}
	value := new(big.Int).SetBytes(body)
	if value.Cmp(big.NewInt(1)) > 0 {
		return false, errors.New("invalid authorization-state result")
	}
	return value.Sign() != 0, nil
}
