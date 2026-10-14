package sandbox

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"net/url"
	"strings"
	"time"

	"github.com/decred/dcrd/dcrec/secp256k1/v4"
	"github.com/decred/dcrd/dcrec/secp256k1/v4/ecdsa"
	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/sha3"
)

const baseSepoliaChainID int64 = 84532

type WalletChallenge struct {
	ID          string    `json:"id"`
	Address     string    `json:"address"`
	ChainID     int64     `json:"chain_id"`
	Message     string    `json:"message"`
	ExpiresAt   time.Time `json:"expires_at"`
	Designation string    `json:"designation"`
}

type WalletBinding struct {
	ID          string    `json:"id"`
	Address     string    `json:"address"`
	ChainID     int64     `json:"chain_id"`
	Epoch       int64     `json:"epoch"`
	ProofHash   string    `json:"proof_hash"`
	BoundAt     time.Time `json:"bound_at"`
	Designation string    `json:"designation"`
}

var ErrNotFound = errors.New("sandbox resource not found")

func validateFinancialOrigin(origin string) (*url.URL, error) {
	u, err := url.Parse(origin)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.Path != "" || u.RawPath != "" || u.RawQuery != "" || u.Fragment != "" || u.Opaque != "" || strings.TrimSpace(origin) != origin {
		return nil, ErrInvalid
	}
	if u.Host != strings.ToLower(u.Host) || u.Hostname() == "" {
		return nil, ErrInvalid
	}
	return u, nil
}

func validateWalletCommandKey(key string) error {
	if len(key) < 1 || len(key) > 128 || strings.TrimSpace(key) != key {
		return ErrInvalid
	}
	for _, c := range key {
		if c < 0x21 || c > 0x7e {
			return ErrInvalid
		}
	}
	return nil
}

func normalizeWalletAddress(address string) (string, error) {
	if len(address) != 42 || !strings.HasPrefix(address, "0x") {
		return "", ErrInvalid
	}
	decoded, err := hex.DecodeString(address[2:])
	if err != nil || len(decoded) != 20 {
		return "", ErrInvalid
	}
	lower := strings.ToLower(address)
	upperBody := strings.ToUpper(address[2:])
	if address != lower && address[2:] != upperBody && checksumWalletAddress(lower) != address {
		return "", ErrInvalid
	}
	return lower, nil
}

func checksumWalletAddress(lower string) string {
	body := strings.TrimPrefix(strings.ToLower(lower), "0x")
	hash := walletKeccak256([]byte(body))
	var out strings.Builder
	out.Grow(42)
	out.WriteString("0x")
	for i, c := range body {
		if c >= 'a' && c <= 'f' {
			nibble := hash[i/2] & 0x0f
			if i%2 == 0 {
				nibble = hash[i/2] >> 4
			}
			if nibble >= 8 {
				c -= 'a' - 'A'
			}
		}
		out.WriteByte(byte(c))
	}
	return out.String()
}

func walletKeccak256(message []byte) []byte {
	h := sha3.NewLegacyKeccak256()
	_, _ = h.Write(message)
	return h.Sum(nil)
}

func walletSIWEMessage(origin *url.URL, actorID, address, challengeID, nonce string, epoch int64, issuedAt, expiresAt time.Time) string {
	statement := fmt.Sprintf("Prove control of this wallet for Undergrid sandbox actor %s at binding epoch %d. This proves control only; it does not authorize payment.", actorID, epoch)
	return fmt.Sprintf("%s://%s wants you to sign in with your Ethereum account:\n%s\n\n%s\n\nURI: %s/v1/owner/wallet-binding\nVersion: 1\nChain ID: %d\nNonce: %s\nIssued At: %s\nExpiration Time: %s\nRequest ID: %s\nResources:\n- urn:undergrid:sandbox:wallet-control",
		origin.Scheme, origin.Host, checksumWalletAddress(address), statement, origin.String(), baseSepoliaChainID, nonce,
		issuedAt.UTC().Format(time.RFC3339Nano), expiresAt.UTC().Format(time.RFC3339Nano), challengeID)
}

func walletRequestHash(address string) string {
	h := sha256.Sum256([]byte("wallet-control|" + address + "|84532"))
	return hex.EncodeToString(h[:])
}

func walletProofRequestHash(challengeID string, signature []byte) string {
	h := sha256.New()
	_, _ = h.Write([]byte("wallet-control-proof|" + challengeID + "|"))
	_, _ = h.Write(signature)
	return hex.EncodeToString(h.Sum(nil))
}

func signatureDigest(signature []byte) string {
	h := sha256.Sum256(signature)
	return hex.EncodeToString(h[:])
}

func parseWalletSignature(signature string) ([]byte, error) {
	if len(signature) != 132 || !strings.HasPrefix(signature, "0x") {
		return nil, ErrInvalid
	}
	raw, err := hex.DecodeString(signature[2:])
	if err != nil || len(raw) != 65 {
		return nil, ErrInvalid
	}
	return raw, nil
}

func recoverWalletAddress(message string, signature []byte) (string, error) {
	if len(signature) != 65 {
		return "", ErrInvalid
	}
	v := signature[64]
	if v >= 27 {
		v -= 27
	}
	if v > 1 {
		return "", ErrInvalid
	}
	r := new(big.Int).SetBytes(signature[:32])
	s := new(big.Int).SetBytes(signature[32:64])
	n := secp256k1.Params().N
	if r.Sign() <= 0 || r.Cmp(n) >= 0 || s.Sign() <= 0 || s.Cmp(new(big.Int).Rsh(new(big.Int).Set(n), 1)) > 0 {
		return "", ErrInvalid
	}
	preimage := []byte(fmt.Sprintf("\x19Ethereum Signed Message:\n%d%s", len(message), message))
	digest := walletKeccak256(preimage)
	compact := make([]byte, 65)
	compact[0] = 27 + v
	copy(compact[1:], signature[:64])
	publicKey, _, err := ecdsa.RecoverCompact(compact, digest)
	if err != nil {
		return "", ErrInvalid
	}
	serialized := publicKey.SerializeUncompressed()
	addressHash := walletKeccak256(serialized[1:])
	return "0x" + hex.EncodeToString(addressHash[12:]), nil
}

func randomWalletNonce() (string, error) {
	value := make([]byte, 32)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return hex.EncodeToString(value), nil
}

func readWalletChallenge(ctx context.Context, tx pgx.Tx, id, actorID string) (WalletChallenge, error) {
	var out WalletChallenge
	err := tx.QueryRow(ctx, `SELECT id::text,address,chain_id,message,expires_at
		FROM sandbox_wallet_challenges WHERE id=$1 AND actor_id=$2`, id, actorID).Scan(
		&out.ID, &out.Address, &out.ChainID, &out.Message, &out.ExpiresAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return out, ErrUnauthorized
	}
	out.ExpiresAt = out.ExpiresAt.UTC()
	out.Designation = "TEST — WALLET CONTROL ONLY"
	return out, err
}

func readWalletBinding(ctx context.Context, tx pgx.Tx, id string) (WalletBinding, error) {
	var out WalletBinding
	err := tx.QueryRow(ctx, `SELECT id::text,address,chain_id,epoch,proof_hash,bound_at
		FROM sandbox_wallet_challenges WHERE id=$1 AND state='bound'`, id).Scan(
		&out.ID, &out.Address, &out.ChainID, &out.Epoch, &out.ProofHash, &out.BoundAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return WalletBinding{}, ErrNotFound
	}
	out.BoundAt = out.BoundAt.UTC()
	out.Designation = "TEST — WALLET CONTROL ONLY"
	return out, err
}

func (s *Store) CreateWalletChallenge(ctx context.Context, owner Owner, key, address, financialOrigin string) (WalletChallenge, error) {
	var result WalletChallenge
	if err := validateWalletCommandKey(key); err != nil {
		return result, err
	}
	normalizedAddress, err := normalizeWalletAddress(address)
	if err != nil {
		return result, err
	}
	origin, err := validateFinancialOrigin(financialOrigin)
	if err != nil {
		return result, err
	}
	requestHash := walletRequestHash(normalizedAddress)
	id := randomID()
	nonce, err := randomWalletNonce()
	if err != nil {
		return result, err
	}
	return result, s.transaction(ctx, "undergrid_sandbox_app", func(tx pgx.Tx) error {
		a, err := ownerContext(ctx, tx, owner)
		if err != nil {
			return err
		}
		var oldID, oldHash string
		oldErr := tx.QueryRow(ctx, `SELECT id::text,issue_request_hash FROM sandbox_wallet_challenges
			WHERE actor_id=$1 AND issue_key=$2`, a.actor, key).Scan(&oldID, &oldHash)
		if oldErr == nil {
			if oldHash != requestHash {
				return ErrConflict
			}
			result, err = readWalletChallenge(ctx, tx, oldID, a.actor)
			return err
		}
		if !errors.Is(oldErr, pgx.ErrNoRows) {
			return oldErr
		}
		var epoch int64
		if err = tx.QueryRow(ctx, "SELECT wallet_binding_epoch FROM sandbox_actors WHERE id=$1", a.actor).Scan(&epoch); err != nil {
			return err
		}
		var issuedAt time.Time
		if err = tx.QueryRow(ctx, "SELECT clock_timestamp()").Scan(&issuedAt); err != nil {
			return err
		}
		issuedAt = issuedAt.UTC().Truncate(time.Microsecond)
		ttl := s.walletChallengeTTL
		if ttl <= 0 {
			ttl = 10 * time.Minute
		}
		expiresAt := issuedAt.Add(ttl)
		message := walletSIWEMessage(origin, a.actor, normalizedAddress, id, nonce, epoch, issuedAt, expiresAt)
		if _, err = tx.Exec(ctx, `UPDATE sandbox_wallet_challenges SET state='superseded'
			WHERE actor_id=$1 AND state='issued'`, a.actor); err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `INSERT INTO sandbox_wallet_challenges
			(id,actor_id,address,chain_id,expected_epoch,nonce,message,issue_key,issue_request_hash,issued_at,expires_at)
			VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`, id, a.actor, normalizedAddress,
			baseSepoliaChainID, epoch, nonce, message, key, requestHash, issuedAt, expiresAt)
		if err != nil {
			return err
		}
		result = WalletChallenge{ID: id, Address: normalizedAddress, ChainID: baseSepoliaChainID,
			Message: message, ExpiresAt: expiresAt.UTC(), Designation: "TEST — WALLET CONTROL ONLY"}
		return nil
	})
}

func (s *Store) ProveWalletControl(ctx context.Context, owner Owner, id, key, signature, financialOrigin string) (WalletBinding, error) {
	var result WalletBinding
	if !validUUID(id) {
		return result, ErrInvalid
	}
	if err := validateWalletCommandKey(key); err != nil {
		return result, err
	}
	rawSignature, err := parseWalletSignature(signature)
	if err != nil {
		return result, err
	}
	origin, err := validateFinancialOrigin(financialOrigin)
	if err != nil {
		return result, err
	}
	requestHash := walletProofRequestHash(id, rawSignature)
	proofHash := signatureDigest(rawSignature)
	return result, s.transaction(ctx, "undergrid_sandbox_app", func(tx pgx.Tx) error {
		a, err := ownerContext(ctx, tx, owner)
		if err != nil {
			return err
		}
		var priorID, priorHash string
		priorErr := tx.QueryRow(ctx, `SELECT id::text,proof_request_hash FROM sandbox_wallet_challenges
			WHERE actor_id=$1 AND proof_key=$2 AND state='bound'`, a.actor, key).Scan(&priorID, &priorHash)
		if priorErr == nil {
			if priorID != id || priorHash != requestHash {
				return ErrConflict
			}
			result, err = readWalletBinding(ctx, tx, priorID)
			return err
		}
		if !errors.Is(priorErr, pgx.ErrNoRows) {
			return priorErr
		}
		var state, oldKey, oldRequestHash string
		var address, message string
		var nonce string
		var chainID int64
		var expectedEpoch int64
		var expiresAt, issuedAt time.Time
		err = tx.QueryRow(ctx, `SELECT state,COALESCE(proof_key,''),COALESCE(proof_request_hash,''),
			address,message,chain_id,expected_epoch,issued_at,expires_at,nonce FROM sandbox_wallet_challenges
			WHERE id=$1 AND actor_id=$2 FOR UPDATE`, id, a.actor).Scan(
			&state, &oldKey, &oldRequestHash, &address, &message, &chainID, &expectedEpoch, &issuedAt, &expiresAt, &nonce)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrUnauthorized
		}
		if err != nil {
			return err
		}
		if state == "bound" {
			if oldKey == key && oldRequestHash == requestHash {
				result, err = readWalletBinding(ctx, tx, id)
				return err
			}
			return ErrConflict
		}
		if state != "issued" {
			return ErrConflict
		}
		var currentEpoch int64
		if err = tx.QueryRow(ctx, "SELECT wallet_binding_epoch FROM sandbox_actors WHERE id=$1", a.actor).Scan(&currentEpoch); err != nil {
			return err
		}
		if currentEpoch != expectedEpoch {
			return ErrConflict
		}
		var now time.Time
		if err = tx.QueryRow(ctx, "SELECT clock_timestamp()").Scan(&now); err != nil {
			return err
		}
		if !expiresAt.After(now) {
			return ErrInvalid
		}
		if chainID != baseSepoliaChainID {
			return ErrInvalid
		}
		if message != walletSIWEMessage(origin, a.actor, address, id, nonce, expectedEpoch, issuedAt, expiresAt) {
			return ErrConflict
		}
		recovered, err := recoverWalletAddress(message, rawSignature)
		if err != nil || recovered != address {
			return ErrInvalid
		}
		var boundAt time.Time
		epoch := expectedEpoch + 1
		err = tx.QueryRow(ctx, `UPDATE sandbox_wallet_challenges SET state='bound',proof_key=$2,
			proof_request_hash=$3,proof_hash=$4,epoch=$5,bound_at=clock_timestamp()
			WHERE id=$1 AND actor_id=$6 AND state='issued' RETURNING bound_at`,
			id, key, requestHash, proofHash, epoch, a.actor).Scan(&boundAt)
		if err != nil {
			return err
		}
		updated, err := tx.Exec(ctx, `UPDATE sandbox_actors SET wallet_binding_id=$2,wallet_binding_epoch=$3
			WHERE id=$1 AND wallet_binding_epoch=$4`, a.actor, id, epoch, expectedEpoch)
		if err != nil {
			return err
		}
		if updated.RowsAffected() != 1 {
			return ErrConflict
		}
		result = WalletBinding{ID: id, Address: address, ChainID: chainID, Epoch: epoch,
			ProofHash: proofHash, BoundAt: boundAt.UTC(), Designation: "TEST — WALLET CONTROL ONLY"}
		return nil
	})
}

func (s *Store) GetWalletBinding(ctx context.Context, owner Owner) (WalletBinding, error) {
	var result WalletBinding
	err := s.transaction(ctx, "undergrid_sandbox_app", func(tx pgx.Tx) error {
		a, err := ownerContext(ctx, tx, owner)
		if err != nil {
			return err
		}
		var id *string
		if err = tx.QueryRow(ctx, "SELECT wallet_binding_id::text FROM sandbox_actors WHERE id=$1", a.actor).Scan(&id); err != nil {
			return err
		}
		if id == nil {
			return ErrNotFound
		}
		result, err = readWalletBinding(ctx, tx, *id)
		return err
	})
	return result, err
}
