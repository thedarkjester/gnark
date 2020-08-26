package r1cs

import (
	backend_bls377 "github.com/consensys/gnark/backend/bls377"
	backend_bls381 "github.com/consensys/gnark/backend/bls381"
	backend_bn256 "github.com/consensys/gnark/backend/bn256"
	"github.com/consensys/gnark/encoding"
	"github.com/consensys/gurvy"
)

// R1CS represents a rank 1 constraint system
// it's underlying implementation is curve specific (i.e bn256/R1CS, ...)
type R1CS interface {
	Inspect(solution map[string]interface{}, showsInputs bool) (map[string]interface{}, error)
	GetNbConstraints() int
}

// Read ...
// TODO likely temporary method, need a clean up pass on serialization things
func Read(path string) (R1CS, error) {
	curveID, err := encoding.PeekCurveID(path)
	if err != nil {
		return nil, err
	}
	var r1cs R1CS
	switch curveID {
	case gurvy.BN256:
		r1cs = &backend_bn256.R1CS{}
	case gurvy.BLS377:
		r1cs = &backend_bls377.R1CS{}
	case gurvy.BLS381:
		r1cs = &backend_bls381.R1CS{}
	default:
		panic("not implemented")
	}

	if err := encoding.Read(path, r1cs, curveID); err != nil {
		return nil, err
	}
	return r1cs, err
}