// Copyright 2020 ConsenSys Software Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Code generated by gnark DO NOT EDIT

package cs

import (
	"errors"
	"fmt"
	"github.com/consensys/gnark-crypto/ecc"
	"github.com/fxamacker/cbor/v2"
	"io"
	"time"

	"github.com/consensys/gnark/backend/witness"
	"github.com/consensys/gnark/constraint"
	"github.com/consensys/gnark/constraint/solver"
	"github.com/consensys/gnark/internal/backend/ioutils"
	"github.com/consensys/gnark/logger"

	"github.com/consensys/gnark-crypto/ecc/bls12-377/fr"
)

// SparseR1CS represents a Plonk like circuit
type SparseR1CS struct {
	constraint.NEWCS
	CoeffTable
	arithEngine
}

// NewSparseR1CS returns a new SparseR1CS and sets r1cs.Coefficient (fr.Element) from provided big.Int values
func NewSparseR1CS(capacity int) *SparseR1CS {
	cs := SparseR1CS{
		NEWCS: constraint.NEWCS{
			System:       constraint.NewSystem(fr.Modulus()),
			Instructions: make([]constraint.Instruction, 0, capacity),
			CallData:     make([]uint32, 0, capacity*2),
		},
		CoeffTable: newCoeffTable(capacity / 10),
	}

	return &cs
}

// func (cs *SparseR1CS) AddConstraint(c constraint.SparseR1C, debugInfo ...constraint.DebugInfo) int {
// 	profile.RecordConstraint()
// 	cs.Constraints = append(cs.Constraints, c)
// 	cID := len(cs.Constraints) - 1
// 	if len(debugInfo) == 1 {
// 		cs.DebugInfo = append(cs.DebugInfo, constraint.LogEntry(debugInfo[0]))
// 		cs.MDebug[cID] = len(cs.DebugInfo) - 1
// 	}
// 	cs.UpdateLevel(cID, &c)

// 	return cID
// }

func (c *SparseR1CS) Solve(witness witness.Witness, opts ...solver.Option) (any, error) {
	opt, err := solver.NewConfig(opts...)
	if err != nil {
		return nil, err
	}

	// compute the constraint system solution
	var solution []fr.Element
	if solution, err = c.solve(witness.Vector().(fr.Vector), opt); err != nil {
		return nil, err
	}

	var res SparseR1CSSolution
	// query l, r, o in Lagrange basis, not blinded
	res.L, res.R, res.O = c.evaluateLROSmallDomain(solution)

	return &res, nil
}

// evaluateLROSmallDomain extracts the solution l, r, o, and returns it in lagrange form.
// solution = [ public | secret | internal ]
func (cs *SparseR1CS) evaluateLROSmallDomain(solution []fr.Element) ([]fr.Element, []fr.Element, []fr.Element) {

	//s := int(pk.Domain[0].Cardinality)
	s := cs.GetNbConstraints() + len(cs.Public) // len(spr.Public) is for the placeholder constraints
	s = int(ecc.NextPowerOfTwo(uint64(s)))

	var l, r, o []fr.Element
	l = make([]fr.Element, s)
	r = make([]fr.Element, s)
	o = make([]fr.Element, s)
	s0 := solution[0]

	for i := 0; i < len(cs.Public); i++ { // placeholders
		l[i] = solution[i]
		r[i] = s0
		o[i] = s0
	}
	offset := len(cs.Public)
	nbConstraints := cs.GetNbConstraints()

	var sparseR1C constraint.SparseR1C
	j := 0
	for _, inst := range cs.Instructions {
		blueprint := cs.Blueprints[inst.BlueprintID]
		if bc, ok := blueprint.(constraint.BlueprintSparseR1C); ok {
			calldata := cs.CallData[inst.StartCallData : inst.StartCallData+uint64(blueprint.NbInputs())]
			bc.DecompressSparseR1C(&sparseR1C, calldata)

			l[offset+j] = solution[sparseR1C.L.WireID()]
			r[offset+j] = solution[sparseR1C.R.WireID()]
			o[offset+j] = solution[sparseR1C.O.WireID()]
			j++
		} else {
			panic("not implemented")
		}
	}

	offset += nbConstraints

	for i := 0; i < s-offset; i++ { // offset to reach 2**n constraints (where the id of l,r,o is 0, so we assign solution[0])
		l[offset+i] = s0
		r[offset+i] = s0
		o[offset+i] = s0
	}

	return l, r, o

}

// Solve sets all the wires.
// solution.values =  [publicInputs | secretInputs | internalVariables ]
// witness: contains the input variables
// it returns the full slice of wires
func (cs *SparseR1CS) solve(witness fr.Vector, opt solver.Config) (fr.Vector, error) {
	log := logger.Logger().With().Int("nbConstraints", cs.GetNbConstraints()).Str("backend", "plonk").Logger()

	// set the slices holding the solution.values and monitoring which variables have been solved
	nbVariables := cs.NbInternalVariables + len(cs.Secret) + len(cs.Public)

	start := time.Now()

	expectedWitnessSize := len(cs.Public) + len(cs.Secret)
	if len(witness) != expectedWitnessSize {
		return make(fr.Vector, nbVariables), fmt.Errorf(
			"invalid witness size, got %d, expected %d = %d (public) + %d (secret)",
			len(witness),
			expectedWitnessSize,
			len(cs.Public),
			len(cs.Secret),
		)
	}

	// keep track of wire that have a value
	solution, err := newSolution(&cs.System, nbVariables, opt.HintFunctions, cs.Coefficients)
	if err != nil {
		return solution.values, err
	}

	// solution.values = [publicInputs | secretInputs | internalVariables ] -> we fill publicInputs | secretInputs
	copy(solution.values, witness)
	for i := 0; i < len(witness); i++ {
		solution.solved[i] = true
	}

	// keep track of the number of wire instantiations we do, for a sanity check to ensure
	// we instantiated all wires
	solution.nbSolved += uint64(len(witness))

	// defer log printing once all solution.values are computed
	defer solution.printLogs(opt.Logger, cs.Logs)

	// batch invert the coefficients to avoid many divisions in the solver
	coefficientsNegInv := fr.BatchInvert(cs.Coefficients)
	for i := 0; i < len(coefficientsNegInv); i++ {
		coefficientsNegInv[i].Neg(&coefficientsNegInv[i])
	}

	// solve
	j := 0
	var sparseR1C constraint.SparseR1C

	for _, inst := range cs.Instructions {
		blueprint := cs.Blueprints[inst.BlueprintID]
		if bc, ok := blueprint.(constraint.BlueprintSparseR1C); ok {
			calldata := cs.CallData[inst.StartCallData : inst.StartCallData+uint64(blueprint.NbInputs())]
			bc.DecompressSparseR1C(&sparseR1C, calldata)

			if err := cs.solveConstraint(sparseR1C, &solution, coefficientsNegInv); err != nil {
				return solution.values, &UnsatisfiedConstraintError{CID: j, Err: err}
			}
			if err := cs.checkConstraint(sparseR1C, &solution); err != nil {
				// if dID, ok := cs.MDebug[i]; ok {
				// 	errMsg := solution.logValue(cs.DebugInfo[dID])
				// 	chError <- &UnsatisfiedConstraintError{CID: i, DebugInfo: &errMsg}
				// } else {
				// 	chError <- &UnsatisfiedConstraintError{CID: i, Err: err}
				// }
				return solution.values, &UnsatisfiedConstraintError{CID: j, Err: err}
			}

			// b.SolveFor(k, cs.Instructions[i], &solution, cs)
			j++
		} else {
			panic("not implemented")
		}
	}

	// if err := cs.parallelSolve(&solution, coefficientsNegInv); err != nil {
	// 	if unsatisfiedErr, ok := err.(*UnsatisfiedConstraintError); ok {
	// 		log.Err(errors.New("unsatisfied constraint")).Int("id", unsatisfiedErr.CID).Send()
	// 	} else {
	// 		log.Err(err).Send()
	// 	}
	// 	return solution.values, err
	// }

	// sanity check; ensure all wires are marked as "instantiated"
	if !solution.isValid() {
		log.Err(errors.New("solver didn't instantiate all wires")).Send()
		panic("solver didn't instantiate all wires")
	}

	log.Debug().Dur("took", time.Since(start)).Msg("constraint system solver done")

	return solution.values, nil

}

// func (cs *SparseR1CS) parallelSolve(solution *solution, coefficientsNegInv fr.Vector) error {
// 	// minWorkPerCPU is the minimum target number of constraint a task should hold
// 	// in other words, if a level has less than minWorkPerCPU, it will not be parallelized and executed
// 	// sequentially without sync.
// 	const minWorkPerCPU = 50.0

// 	// cs.Levels has a list of levels, where all constraints in a level l(n) are independent
// 	// and may only have dependencies on previous levels

// 	var wg sync.WaitGroup
// 	chTasks := make(chan []int, runtime.NumCPU())
// 	chError := make(chan *UnsatisfiedConstraintError, runtime.NumCPU())

// 	// start a worker pool
// 	// each worker wait on chTasks
// 	// a task is a slice of constraint indexes to be solved
// 	for i := 0; i < runtime.NumCPU(); i++ {
// 		go func() {
// 			for t := range chTasks {
// 				for _, i := range t {
// 					// for each constraint in the task, solve it.
// 					if err := cs.solveConstraint(cs.Constraints[i], solution, coefficientsNegInv); err != nil {
// 						chError <- &UnsatisfiedConstraintError{CID: i, Err: err}
// 						wg.Done()
// 						return
// 					}
// 					if err := cs.checkConstraint(cs.Constraints[i], solution); err != nil {
// 						if dID, ok := cs.MDebug[i]; ok {
// 							errMsg := solution.logValue(cs.DebugInfo[dID])
// 							chError <- &UnsatisfiedConstraintError{CID: i, DebugInfo: &errMsg}
// 						} else {
// 							chError <- &UnsatisfiedConstraintError{CID: i, Err: err}
// 						}
// 						wg.Done()
// 						return
// 					}
// 				}
// 				wg.Done()
// 			}
// 		}()
// 	}

// 	// clean up pool go routines
// 	defer func() {
// 		close(chTasks)
// 		close(chError)
// 	}()

// 	// for each level, we push the tasks
// 	for _, level := range cs.Levels {

// 		// max CPU to use
// 		maxCPU := float64(len(level)) / minWorkPerCPU

// 		if maxCPU <= 1.0 {
// 			// we do it sequentially
// 			for _, i := range level {
// 				if err := cs.solveConstraint(cs.Constraints[i], solution, coefficientsNegInv); err != nil {
// 					return &UnsatisfiedConstraintError{CID: i, Err: err}
// 				}
// 				if err := cs.checkConstraint(cs.Constraints[i], solution); err != nil {
// 					if dID, ok := cs.MDebug[i]; ok {
// 						errMsg := solution.logValue(cs.DebugInfo[dID])
// 						return &UnsatisfiedConstraintError{CID: i, DebugInfo: &errMsg}
// 					}
// 					return &UnsatisfiedConstraintError{CID: i, Err: err}
// 				}
// 			}
// 			continue
// 		}

// 		// number of tasks for this level is set to number of CPU
// 		// but if we don't have enough work for all our CPU, it can be lower.
// 		nbTasks :=  runtime.NumCPU()
// 		maxTasks := int(math.Ceil(maxCPU))
// 		if nbTasks > maxTasks {
// 			nbTasks = maxTasks
// 		}
// 		nbIterationsPerCpus := len(level) / nbTasks

// 		// more CPUs than tasks: a CPU will work on exactly one iteration
// 		// note: this depends on minWorkPerCPU constant
// 		if nbIterationsPerCpus < 1 {
// 			nbIterationsPerCpus = 1
// 			nbTasks = len(level)
// 		}

// 		extraTasks := len(level) - (nbTasks * nbIterationsPerCpus)
// 		extraTasksOffset := 0

// 		for i := 0; i < nbTasks; i++ {
// 			wg.Add(1)
// 			_start := i*nbIterationsPerCpus + extraTasksOffset
// 			_end := _start + nbIterationsPerCpus
// 			if extraTasks > 0 {
// 				_end++
// 				extraTasks--
// 				extraTasksOffset++
// 			}
// 			// since we're never pushing more than num CPU tasks
// 			// we will never be blocked here
// 			chTasks <- level[_start:_end]
// 		}

// 		// wait for the level to be done
// 		wg.Wait()

// 		if len(chError) > 0 {
// 			return <-chError
// 		}
// 	}

// 	return nil
// }

// computeHints computes wires associated with a hint function, if any
// if there is no remaining wire to solve, returns -1
// else returns the wire position (L -> 0, R -> 1, O -> 2)
func (cs *SparseR1CS) computeHints(c constraint.SparseR1C, solution *solution) (int, error) {
	r := -1
	lID, rID, oID := c.L.WireID(), c.R.WireID(), c.O.WireID()

	if (c.L.CoeffID() != 0 || c.M[0].CoeffID() != 0) && !solution.solved[lID] {
		// check if it's a hint
		if hint, ok := cs.MHints[lID]; ok {
			if err := solution.solveWithHint(lID, hint); err != nil {
				return -1, err
			}
		} else {
			r = 0
		}

	}

	if (c.R.CoeffID() != 0 || c.M[1].CoeffID() != 0) && !solution.solved[rID] {
		// check if it's a hint
		if hint, ok := cs.MHints[rID]; ok {
			if err := solution.solveWithHint(rID, hint); err != nil {
				return -1, err
			}
		} else {
			r = 1
		}
	}

	if (c.O.CoeffID() != 0) && !solution.solved[oID] {
		// check if it's a hint
		if hint, ok := cs.MHints[oID]; ok {
			if err := solution.solveWithHint(oID, hint); err != nil {
				return -1, err
			}
		} else {
			r = 2
		}
	}
	return r, nil
}

// solveConstraint solve any unsolved wire in given constraint and update the solution
// a SparseR1C may have up to one unsolved wire (excluding hints)
// if it doesn't, then this function returns and does nothing
func (cs *SparseR1CS) solveConstraint(c constraint.SparseR1C, solution *solution, coefficientsNegInv fr.Vector) error {

	if c.Commitment == constraint.COMMITTED { // a constraint of the form f_L - PI_2 = 0 or f_L = Comm.
		return nil // these are there for enforcing the correctness of the commitment and can be skipped in solving time
	}

	lro, err := cs.computeHints(c, solution)
	if err != nil {
		return err
	}
	if lro == -1 {
		// no unsolved wire
		// can happen if the constraint contained only hint wires.
		return nil
	}
	if lro == 1 { // we solve for R: u1L+u2R+u3LR+u4O+k=0 => R(u2+u3L)+u1L+u4O+k = 0
		if !solution.solved[c.L.WireID()] {
			panic("L wire should be instantiated when we solve R")
		}
		var u1, u2, u3, den, num, v1, v2 fr.Element
		u3.Mul(&cs.Coefficients[c.M[0].CoeffID()], &cs.Coefficients[c.M[1].CoeffID()])
		u1.Set(&cs.Coefficients[c.L.CoeffID()])
		u2.Set(&cs.Coefficients[c.R.CoeffID()])
		den.Mul(&u3, &solution.values[c.L.WireID()]).Add(&den, &u2)

		v1 = solution.computeTerm(c.L)
		v2 = solution.computeTerm(c.O)
		num.Add(&v1, &v2).Add(&num, &cs.Coefficients[c.K])

		// TODO find a way to do lazy div (/ batch inversion)
		num.Div(&num, &den).Neg(&num)
		solution.set(c.L.WireID(), num)
		return nil
	}

	if lro == 0 { // we solve for L: u1L+u2R+u3LR+u4O+k=0 => L(u1+u3R)+u2R+u4O+k = 0
		if !solution.solved[c.R.WireID()] {
			panic("R wire should be instantiated when we solve L")
		}
		var u1, u2, u3, den, num, v1, v2 fr.Element
		u3.Mul(&cs.Coefficients[c.M[0].CoeffID()], &cs.Coefficients[c.M[1].CoeffID()])
		u1.Set(&cs.Coefficients[c.L.CoeffID()])
		u2.Set(&cs.Coefficients[c.R.CoeffID()])
		den.Mul(&u3, &solution.values[c.R.WireID()]).Add(&den, &u1)

		v1 = solution.computeTerm(c.R)
		v2 = solution.computeTerm(c.O)
		num.Add(&v1, &v2).Add(&num, &cs.Coefficients[c.K])

		// TODO find a way to do lazy div (/ batch inversion)
		num.Div(&num, &den).Neg(&num)
		solution.set(c.L.WireID(), num)
		return nil

	}
	// O we solve for O
	var o fr.Element
	cID, vID := c.O.CoeffID(), c.O.WireID()

	l := solution.computeTerm(c.L)
	r := solution.computeTerm(c.R)
	m0 := solution.computeTerm(c.M[0])
	m1 := solution.computeTerm(c.M[1])

	// o = - ((m0 * m1) + l + r + c.K) / c.O
	o.Mul(&m0, &m1).Add(&o, &l).Add(&o, &r).Add(&o, &cs.Coefficients[c.K])
	o.Mul(&o, &coefficientsNegInv[cID])

	solution.set(vID, o)

	return nil
}

// IsSolved
// Deprecated: use _, err := Solve(...) instead
func (cs *SparseR1CS) IsSolved(witness witness.Witness, opts ...solver.Option) error {
	_, err := cs.Solve(witness, opts...)
	return err
}

// GetConstraints return the list of SparseR1C and a coefficient resolver
func (cs *SparseR1CS) GetConstraints() ([]constraint.SparseR1C, constraint.Resolver) {

	toReturn := make([]constraint.SparseR1C, 0, cs.GetNbConstraints())

	var sparseR1C constraint.SparseR1C
	for _, inst := range cs.Instructions {
		blueprint := cs.Blueprints[inst.BlueprintID]
		if bc, ok := blueprint.(constraint.BlueprintSparseR1C); ok {
			calldata := cs.CallData[inst.StartCallData : inst.StartCallData+uint64(blueprint.NbInputs())]
			bc.DecompressSparseR1C(&sparseR1C, calldata)
		} else {
			panic("not implemented")
		}
	}
	return toReturn, cs
}

func (cs *SparseR1CS) GetCoefficient(i int) (r constraint.Coeff) {
	copy(r[:], cs.Coefficients[i][:])
	return
}

// checkConstraint verifies that the constraint holds
func (cs *SparseR1CS) checkConstraint(c constraint.SparseR1C, solution *solution) error {

	if c.Commitment != constraint.NOT { // a constraint of the form f_L - PI_2 = 0 or f_L = Comm.
		return nil // these are there for enforcing the correctness of the commitment and can be skipped in solving time
	}

	l := solution.computeTerm(c.L)
	r := solution.computeTerm(c.R)
	m0 := solution.computeTerm(c.M[0])
	m1 := solution.computeTerm(c.M[1])
	o := solution.computeTerm(c.O)

	// l + r + (m0 * m1) + o + c.K == 0
	var t fr.Element
	t.Mul(&m0, &m1).Add(&t, &l).Add(&t, &r).Add(&t, &o).Add(&t, &cs.Coefficients[c.K])
	if !t.IsZero() {
		return fmt.Errorf("qL⋅xa + qR⋅xb + qO⋅xc + qM⋅(xaxb) + qC != 0 → %s + %s + %s + (%s × %s) + %s != 0",
			l.String(),
			r.String(),
			o.String(),
			m0.String(),
			m1.String(),
			cs.Coefficients[c.K].String(),
		)
	}
	return nil

}

// GetNbCoefficients return the number of unique coefficients needed in the R1CS
func (cs *SparseR1CS) GetNbCoefficients() int {
	return len(cs.Coefficients)
}

// CurveID returns curve ID as defined in gnark-crypto (ecc.BLS12-377)
func (cs *SparseR1CS) CurveID() ecc.ID {
	return ecc.BLS12_377
}

// WriteTo encodes SparseR1CS into provided io.Writer using cbor
func (cs *SparseR1CS) WriteTo(w io.Writer) (int64, error) {
	_w := ioutils.WriterCounter{W: w} // wraps writer to count the bytes written
	enc, err := cbor.CoreDetEncOptions().EncMode()
	if err != nil {
		return 0, err
	}
	encoder := enc.NewEncoder(&_w)

	// encode our object
	err = encoder.Encode(cs)
	return _w.N, err
}

// ReadFrom attempts to decode SparseR1CS from io.Reader using cbor
func (cs *SparseR1CS) ReadFrom(r io.Reader) (int64, error) {
	dm, err := cbor.DecOptions{
		MaxArrayElements: 134217728,
		MaxMapPairs:      134217728,
	}.DecMode()
	if err != nil {
		return 0, err
	}
	decoder := dm.NewDecoder(r)

	// initialize coeff table
	cs.CoeffTable = newCoeffTable(0)

	if err := decoder.Decode(cs); err != nil {
		return int64(decoder.NumBytesRead()), err
	}

	if err := cs.CheckSerializationHeader(); err != nil {
		return int64(decoder.NumBytesRead()), err
	}

	return int64(decoder.NumBytesRead()), nil
}
