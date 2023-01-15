package direct

import (
	"crypto/ed25519"
	"encoding/hex"
	"fmt"

	sr25519 "github.com/ChainSafe/go-schnorrkel"
	"github.com/pkg/errors"
	"github.com/threefoldtech/rmb-sdk-go/direct/types"
	"github.com/threefoldtech/substrate-client"

	"github.com/gtank/merlin"
	"github.com/rs/zerolog/log"
)

const (
	SignatureTypeEd25519 = "ed25519"
	SignatureTypeSr25519 = "sr25519"
)

type Verifier interface {
	Verify(msg []byte, sig []byte) bool
}

type Ed25519VerifyingKey []byte
type Sr25519VerifyingKey []byte

func (k Ed25519VerifyingKey) Verify(msg []byte, sig []byte) bool {
	return ed25519.Verify([]byte(k), msg, sig)
}

func signingContext(msg []byte) *merlin.Transcript {
	return sr25519.NewSigningContext([]byte("substrate"), msg)
}

func (k Sr25519VerifyingKey) verify(pub sr25519.PublicKey, msg []byte, signature []byte) bool {
	var sigs [64]byte
	copy(sigs[:], signature)
	sig := new(sr25519.Signature)
	if err := sig.Decode(sigs); err != nil {
		return false
	}
	ok, err := pub.Verify(sig, signingContext(msg))
	if err != nil {
		log.Error().Err(err).Msg("failed to verify signature")
		return false
	}

	return ok
}

func (k Sr25519VerifyingKey) pubKey() (*sr25519.PublicKey, error) {
	var pubBytes [32]byte
	copy(pubBytes[:], k)
	pk := new(sr25519.PublicKey)

	if err := pk.Decode(pubBytes); err != nil {
		return nil, err
	}
	return pk, nil
}

func (k Sr25519VerifyingKey) Verify(msg []byte, sig []byte) bool {
	pk, err := k.pubKey()
	if err != nil {
		log.Error().Str("pk", hex.EncodeToString(k)).Err(err).Msg("failed to get sr25519 key from bytes returned from substrate")
		return false
	}
	return k.verify(*pk, msg, sig)
}

func constructVerifier(publicKey []byte, key_type string) (Verifier, error) {
	if key_type == SignatureTypeEd25519 {
		return Ed25519VerifyingKey(publicKey), nil
	} else if key_type == SignatureTypeSr25519 {
		return Sr25519VerifyingKey(publicKey), nil
	} else {
		return nil, fmt.Errorf("unrecognized key type %s", key_type)
	}
}

func sigTypeToChar(sigType string) (byte, error) {
	if sigType == SignatureTypeEd25519 {
		return byte('e'), nil
	} else if sigType == SignatureTypeSr25519 {
		return byte('s'), nil
	} else {
		return 0, fmt.Errorf("unrecognized signature type %s", sigType)
	}
}

func charToSigType(prefix byte) (string, error) {
	if prefix == byte('e') {
		return SignatureTypeEd25519, nil
	} else if prefix == byte('s') {
		return SignatureTypeSr25519, nil
	} else {
		return "", fmt.Errorf("unrecognized signature prefix %x", []byte{prefix})
	}
}

func VerifySignature(sub *substrate.Substrate, env *types.Envelope) error {

	twin, err := sub.GetTwin(env.Source.Twin)
	if err != nil {
		return errors.Wrapf(err, "could not get twin from twin id, twinID: %d", env.Source.Twin)
	}
	pk := twin.Account.PublicKey()

	sig := env.GetSignature()
	if sig == nil {
		return errors.Wrap(err, "could not get signature from envelope")
	}
	decoded, err := hex.DecodeString(string(sig))
	if err != nil {
		return errors.Wrap(err, "could not decode signature")
	}
	signatureType, err := charToSigType(decoded[0])
	if err != nil {
		return errors.Wrap(err, "got bad signature type should be either Ed25519 or Sr25519")
	}
	verifier, err := constructVerifier(pk, signatureType)
	if err != nil {
		return err
	}
	data, err := Challenge(env)
	if err != nil {
		return errors.Wrap(err, "could not get challenge hash")
	}
	if !verifier.Verify(data, decoded[1:]) {
		return fmt.Errorf("could not verify signature")
	}
	return nil
}