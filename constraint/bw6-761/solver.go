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
	"github.com/consensys/gnark-crypto/field/pool"
	"github.com/consensys/gnark/constraint"
	csolver "github.com/consensys/gnark/constraint/solver"
	"github.com/rs/zerolog"
	"math"
	"math/big"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/consensys/gnark-crypto/ecc/bw6-761/fr"
)

// solver represent the state of the solver during a call to System.Solve(...)
type solver struct {
	*system

	// values and solved are index by the wire (variable) id
	values   []fr.Element
	solved   []bool
	nbSolved uint64

	// maps hintID to hint function
	mHintsFunctions map[csolver.HintID]csolver.Hint

	// used to out api.Println
	logger zerolog.Logger

	a, b, c            fr.Vector // R1CS solver will compute the a,b,c matrices
	coefficientsNegInv fr.Vector // TODO @gbotrel this should be computed once SparseR1CS solver perf.
}

func newSolver(cs *system, witness fr.Vector, opts ...csolver.Option) (*solver, error) {
	// parse options
	opt, err := csolver.NewConfig(opts...)
	if err != nil {
		return nil, err
	}

	// check witness size
	witnessOffset := 0
	if cs.Type == constraint.SystemR1CS {
		witnessOffset++
	}

	nbWires := len(cs.Public) + len(cs.Secret) + cs.NbInternalVariables
	expectedWitnessSize := len(cs.Public) - witnessOffset + len(cs.Secret)

	if len(witness) != expectedWitnessSize {
		return nil, fmt.Errorf("invalid witness size, got %d, expected %d", len(witness), expectedWitnessSize)
	}

	// check all hints are there
	hintFunctions := opt.HintFunctions

	// hintsDependencies is from compile time; it contains the list of hints the solver **needs**
	var missing []string
	for hintUUID, hintID := range cs.MHintsDependencies {
		if _, ok := hintFunctions[hintUUID]; !ok {
			missing = append(missing, hintID)
		}
	}

	if len(missing) > 0 {
		return nil, fmt.Errorf("solver missing hint(s): %v", missing)
	}

	s := solver{
		system:          cs,
		values:          make([]fr.Element, nbWires),
		solved:          make([]bool, nbWires),
		mHintsFunctions: hintFunctions,
		logger:          opt.Logger,
	}

	// set the witness indexes as solved
	if witnessOffset == 1 {
		s.solved[0] = true // ONE_WIRE
		s.values[0].SetOne()
	}
	copy(s.values[witnessOffset:], witness)
	for i := range witness {
		s.solved[i+witnessOffset] = true
	}

	// keep track of the number of wire instantiations we do, for a post solve sanity check
	// to ensure we instantiated all wires
	s.nbSolved += uint64(len(witness) + witnessOffset)

	if s.Type == constraint.SystemR1CS {
		n := ecc.NextPowerOfTwo(uint64(cs.GetNbConstraints()))
		s.a = make(fr.Vector, cs.GetNbConstraints(), n)
		s.b = make(fr.Vector, cs.GetNbConstraints(), n)
		s.c = make(fr.Vector, cs.GetNbConstraints(), n)
	} else {
		// TODO @gbotrel this could be done once in the CS, most of the time.
		s.coefficientsNegInv = fr.Vector(fr.BatchInvert(s.Coefficients))
		for i := 0; i < len(s.coefficientsNegInv); i++ {
			s.coefficientsNegInv[i].Neg(&s.coefficientsNegInv[i])
		}
	}

	return &s, nil
}

func (s *solver) set(id int, value fr.Element) {
	if s.solved[id] {
		panic("solving the same wire twice should never happen.")
	}
	s.values[id] = value
	s.solved[id] = true
	atomic.AddUint64(&s.nbSolved, 1)
}

// computeTerm computes coeff*variable
// TODO @gbotrel check if t is a Constant only
func (s *solver) computeTerm(t constraint.Term) fr.Element {
	cID, vID := t.CoeffID(), t.WireID()
	if cID != 0 && !s.solved[vID] {
		panic("computing a term with an unsolved wire")
	}
	switch cID {
	case constraint.CoeffIdZero:
		return fr.Element{}
	case constraint.CoeffIdOne:
		return s.values[vID]
	case constraint.CoeffIdTwo:
		var res fr.Element
		res.Double(&s.values[vID])
		return res
	case constraint.CoeffIdMinusOne:
		var res fr.Element
		res.Neg(&s.values[vID])
		return res
	default:
		var res fr.Element
		res.Mul(&s.Coefficients[cID], &s.values[vID])
		return res
	}
}

// r += (t.coeff*t.value)
// TODO @gbotrel check t.IsConstant on the caller side when necessary
func (s *solver) accumulateInto(t constraint.Term, r *fr.Element) {
	cID := t.CoeffID()
	vID := t.WireID()

	switch cID {
	case constraint.CoeffIdZero:
		return
	case constraint.CoeffIdOne:
		r.Add(r, &s.values[vID])
	case constraint.CoeffIdTwo:
		var res fr.Element
		res.Double(&s.values[vID])
		r.Add(r, &res)
	case constraint.CoeffIdMinusOne:
		r.Sub(r, &s.values[vID])
	default:
		var res fr.Element
		res.Mul(&s.Coefficients[cID], &s.values[vID])
		r.Add(r, &res)
	}
}

// solveWithHint executes a hint and assign the result to its defined outputs.
func (s *solver) solveWithHint(h constraint.HintMapping) error {
	// ensure hint function was provided
	f, ok := s.mHintsFunctions[h.HintID]
	if !ok {
		return errors.New("missing hint function")
	}

	// tmp IO big int memory
	nbInputs := len(h.Inputs)
	nbOutputs := len(h.Outputs)
	inputs := make([]*big.Int, nbInputs)
	outputs := make([]*big.Int, nbOutputs)
	for i := 0; i < nbOutputs; i++ {
		outputs[i] = pool.BigInt.Get()
		outputs[i].SetUint64(0)
	}

	q := fr.Modulus()

	for i := 0; i < nbInputs; i++ {
		var v fr.Element
		for _, term := range h.Inputs[i] {
			if term.IsConstant() {
				v.Add(&v, &s.Coefficients[term.CoeffID()])
				continue
			}
			s.accumulateInto(term, &v)
		}
		inputs[i] = pool.BigInt.Get()
		v.BigInt(inputs[i])
	}

	err := f(q, inputs, outputs)

	var v fr.Element
	for i := range outputs {
		v.SetBigInt(outputs[i])
		s.set(h.Outputs[i], v)
		pool.BigInt.Put(outputs[i])
	}

	for i := range inputs {
		pool.BigInt.Put(inputs[i])
	}

	return err
}

func (s *solver) printLogs(logs []constraint.LogEntry) {
	if s.logger.GetLevel() == zerolog.Disabled {
		return
	}

	for i := 0; i < len(logs); i++ {
		logLine := s.logValue(logs[i])
		s.logger.Debug().Str(zerolog.CallerFieldName, logs[i].Caller).Msg(logLine)
	}
}

const unsolvedVariable = "<unsolved>"

func (s *solver) logValue(log constraint.LogEntry) string {
	var toResolve []interface{}
	var (
		eval         fr.Element
		missingValue bool
	)
	for j := 0; j < len(log.ToResolve); j++ {
		// before eval le

		missingValue = false
		eval.SetZero()

		for _, t := range log.ToResolve[j] {
			// for each term in the linear expression

			cID, vID := t.CoeffID(), t.WireID()
			if t.IsConstant() {
				// just add the constant
				eval.Add(&eval, &s.Coefficients[cID])
				continue
			}

			if !s.solved[vID] {
				missingValue = true
				break // stop the loop we can't evaluate.
			}

			tv := s.computeTerm(t)
			eval.Add(&eval, &tv)
		}

		// after
		if missingValue {
			toResolve = append(toResolve, unsolvedVariable)
		} else {
			// we have to append our accumulator
			toResolve = append(toResolve, eval.String())
		}

	}
	if len(log.Stack) > 0 {
		var sbb strings.Builder
		for _, lID := range log.Stack {
			location := s.SymbolTable.Locations[lID]
			function := s.SymbolTable.Functions[location.FunctionID]

			sbb.WriteString(function.Name)
			sbb.WriteByte('\n')
			sbb.WriteByte('\t')
			sbb.WriteString(function.Filename)
			sbb.WriteByte(':')
			sbb.WriteString(strconv.Itoa(int(location.Line)))
			sbb.WriteByte('\n')
		}
		toResolve = append(toResolve, sbb.String())
	}
	return fmt.Sprintf(log.Format, toResolve...)
}

// divByCoeff sets res = res / t.Coeff
func (solver *solver) divByCoeff(res *fr.Element, t constraint.Term) {
	cID := t.CoeffID()
	switch cID {
	case constraint.CoeffIdOne:
		return
	case constraint.CoeffIdMinusOne:
		res.Neg(res)
	case constraint.CoeffIdZero:
		panic("division by 0")
	default:
		// this is slow, but shouldn't happen as divByCoeff is called to
		// remove the coeff of an unsolved wire
		// but unsolved wires are (in gnark frontend) systematically set with a coeff == 1 or -1
		res.Div(res, &solver.Coefficients[cID])
	}
}

// Implement constraint.Solver
func (s *solver) GetValue(cID, vID uint32) constraint.Element {
	var r constraint.Element
	e := s.computeTerm(constraint.Term{CID: cID, VID: vID})
	copy(r[:], e[:])
	return r
}
func (s *solver) GetCoeff(cID uint32) constraint.Element {
	var r constraint.Element
	copy(r[:], s.Coefficients[cID][:])
	return r
}
func (s *solver) SetValue(vID uint32, f constraint.Element) {
	s.set(int(vID), fr.Element(f[:]))
}

// UnsatisfiedConstraintError wraps an error with useful metadata on the unsatisfied constraint
type UnsatisfiedConstraintError struct {
	Err       error
	CID       int     // constraint ID
	DebugInfo *string // optional debug info
}

func (r *UnsatisfiedConstraintError) Error() string {
	if r.DebugInfo != nil {
		return fmt.Sprintf("constraint #%d is not satisfied: %s", r.CID, *r.DebugInfo)
	}
	return fmt.Sprintf("constraint #%d is not satisfied: %s", r.CID, r.Err.Error())
}

// processInstruction decodes the instruction and execute blueprint-defined logic.
// an instruction can encode a hint, a custom constraint or a generic constraint.
func (solver *solver) processInstruction(inst constraint.Instruction) error {
	// fetch the blueprint
	blueprint := solver.Blueprints[inst.BlueprintID]
	calldata := solver.GetCallData(inst)

	// blueprint encodes a hint, we execute.
	// TODO @gbotrel may be worth it to move hint logic in blueprint "solve"
	if bc, ok := blueprint.(constraint.BlueprintHint); ok {
		var hm constraint.HintMapping
		bc.DecompressHint(&hm, calldata)
		return solver.solveWithHint(hm)
	}

	// blueprint declared "I know how to solve this."
	if bc, ok := blueprint.(constraint.BlueprintSolvable); ok {
		return bc.Solve(solver, calldata)
	}

	cID := inst.ConstraintOffset // here we have 1 constraint in the instruction only

	if solver.Type == constraint.SystemR1CS {
		if bc, ok := blueprint.(constraint.BlueprintR1C); ok {
			// TODO @gbotrel use pool object here for the R1C
			var tmpR1C constraint.R1C
			bc.DecompressR1C(&tmpR1C, calldata)
			return solver.solveR1C(cID, &tmpR1C)
		}
	} else if solver.Type == constraint.SystemSparseR1CS {
		if bc, ok := blueprint.(constraint.BlueprintSparseR1C); ok {
			// sparse R1CS
			var tmpSparseR1C constraint.SparseR1C
			bc.DecompressSparseR1C(&tmpSparseR1C, calldata)

			if err := solver.solveSparseR1C(&tmpSparseR1C); err != nil {
				return solver.wrapErrWithDebugInfo(cID, err)
			}
			if err := solver.checkSparseR1C(&tmpSparseR1C); err != nil {
				return solver.wrapErrWithDebugInfo(cID, err)
			}
			return nil
		}
	}

	return nil
}

// run runs the solver. it return an error if a constraint is not satisfied or if not all wires
// were instantiated.
func (solver *solver) run() error {
	// minWorkPerCPU is the minimum target number of constraint a task should hold
	// in other words, if a level has less than minWorkPerCPU, it will not be parallelized and executed
	// sequentially without sync.
	const minWorkPerCPU = 50.0 // TODO @gbotrel revisit that with blocks.

	// cs.Levels has a list of levels, where all constraints in a level l(n) are independent
	// and may only have dependencies on previous levels
	// for each constraint
	// we are guaranteed that each R1C contains at most one unsolved wire
	// first we solve the unsolved wire (if any)
	// then we check that the constraint is valid
	// if a[i] * b[i] != c[i]; it means the constraint is not satisfied
	var wg sync.WaitGroup
	chTasks := make(chan []int, runtime.NumCPU())
	chError := make(chan error, runtime.NumCPU())

	// start a worker pool
	// each worker wait on chTasks
	// a task is a slice of constraint indexes to be solved
	for i := 0; i < runtime.NumCPU(); i++ {
		go func() {
			for t := range chTasks {
				for _, i := range t {
					if err := solver.processInstruction(solver.Instructions[i]); err != nil {
						chError <- err
						wg.Done()
						return
					}
				}
				wg.Done()
			}
		}()
	}

	// clean up pool go routines
	defer func() {
		close(chTasks)
		close(chError)
	}()

	// for each level, we push the tasks
	for _, level := range solver.Levels {

		// max CPU to use
		maxCPU := float64(len(level)) / minWorkPerCPU

		if maxCPU <= 1.0 {
			// we do it sequentially
			for _, i := range level {
				if err := solver.processInstruction(solver.Instructions[i]); err != nil {
					return err
				}
			}
			continue
		}

		// number of tasks for this level is set to number of CPU
		// but if we don't have enough work for all our CPU, it can be lower.
		nbTasks := runtime.NumCPU()
		maxTasks := int(math.Ceil(maxCPU))
		if nbTasks > maxTasks {
			nbTasks = maxTasks
		}
		nbIterationsPerCpus := len(level) / nbTasks

		// more CPUs than tasks: a CPU will work on exactly one iteration
		// note: this depends on minWorkPerCPU constant
		if nbIterationsPerCpus < 1 {
			nbIterationsPerCpus = 1
			nbTasks = len(level)
		}

		extraTasks := len(level) - (nbTasks * nbIterationsPerCpus)
		extraTasksOffset := 0

		for i := 0; i < nbTasks; i++ {
			wg.Add(1)
			_start := i*nbIterationsPerCpus + extraTasksOffset
			_end := _start + nbIterationsPerCpus
			if extraTasks > 0 {
				_end++
				extraTasks--
				extraTasksOffset++
			}
			// since we're never pushing more than num CPU tasks
			// we will never be blocked here
			chTasks <- level[_start:_end]
		}

		// wait for the level to be done
		wg.Wait()

		if len(chError) > 0 {
			return <-chError
		}
	}

	if int(solver.nbSolved) != len(solver.values) {
		return errors.New("solver didn't assign a value to all wires")
	}

	return nil
}

func (solver *solver) wrapErrWithDebugInfo(cID uint32, err error) *UnsatisfiedConstraintError {
	var debugInfo *string
	if dID, ok := solver.MDebug[int(cID)]; ok {
		debugInfo = new(string)
		*debugInfo = solver.logValue(solver.DebugInfo[dID])
	}
	return &UnsatisfiedConstraintError{CID: int(cID), Err: err, DebugInfo: debugInfo}
}

// solveR1C compute unsolved wires in the constraint, if any and set the solver accordingly
//
// returns an error if the solver called a hint function that errored
// returns false, nil if there was no wire to solve
// returns true, nil if exactly one wire was solved. In that case, it is redundant to check that
// the constraint is satisfied later.
func (solver *solver) solveR1C(cID uint32, r *constraint.R1C) error {
	a, b, c := &solver.a[cID], &solver.b[cID], &solver.c[cID]

	// the index of the non-zero entry shows if L, R or O has an uninstantiated wire
	// the content is the ID of the wire non instantiated
	var loc uint8

	var termToCompute constraint.Term

	processLExp := func(l constraint.LinearExpression, val *fr.Element, locValue uint8) {
		for _, t := range l {
			vID := t.WireID()

			// wire is already computed, we just accumulate in val
			if solver.solved[vID] {
				solver.accumulateInto(t, val)
				continue
			}

			if loc != 0 {
				panic("found more than one wire to instantiate")
			}
			termToCompute = t
			loc = locValue
		}
	}

	processLExp(r.L, a, 1)
	processLExp(r.R, b, 2)
	processLExp(r.O, c, 3)

	if loc == 0 {
		// there is nothing to solve, may happen if we have an assertion
		// (ie a constraints that doesn't yield any output)
		// or if we solved the unsolved wires with hint functions
		var check fr.Element
		if !check.Mul(a, b).Equal(c) {
			return solver.wrapErrWithDebugInfo(cID, fmt.Errorf("%s ⋅ %s != %s", a.String(), b.String(), c.String()))
		}
		return nil
	}

	// we compute the wire value and instantiate it
	wID := termToCompute.WireID()

	// solver result
	var wire fr.Element

	switch loc {
	case 1:
		if !b.IsZero() {
			wire.Div(c, b).
				Sub(&wire, a)
			a.Add(a, &wire)
		} else {
			// we didn't actually ensure that a * b == c
			var check fr.Element
			if !check.Mul(a, b).Equal(c) {
				return solver.wrapErrWithDebugInfo(cID, fmt.Errorf("%s ⋅ %s != %s", a.String(), b.String(), c.String()))
			}
		}
	case 2:
		if !a.IsZero() {
			wire.Div(c, a).
				Sub(&wire, b)
			b.Add(b, &wire)
		} else {
			var check fr.Element
			if !check.Mul(a, b).Equal(c) {
				return solver.wrapErrWithDebugInfo(cID, fmt.Errorf("%s ⋅ %s != %s", a.String(), b.String(), c.String()))
			}
		}
	case 3:
		wire.Mul(a, b).
			Sub(&wire, c)

		c.Add(c, &wire)
	}

	// wire is the term (coeff * value)
	// but in the solver we want to store the value only
	// note that in gnark frontend, coeff here is always 1 or -1
	solver.divByCoeff(&wire, termToCompute)
	solver.set(wID, wire)

	return nil
}

// checkSparseR1C verifies that the constraint holds
func (solver *solver) checkSparseR1C(c *constraint.SparseR1C) error {

	if c.Commitment != constraint.NOT { // a constraint of the form f_L - PI_2 = 0 or f_L = Comm.
		return nil // these are there for enforcing the correctness of the commitment and can be skipped in solving time
	}

	l := solver.computeTerm(constraint.Term{CID: c.QL, VID: c.XA})
	r := solver.computeTerm(constraint.Term{CID: c.QR, VID: c.XB})
	m0 := solver.computeTerm(constraint.Term{CID: c.QM, VID: c.XA})
	m1 := solver.values[c.XB]

	o := solver.computeTerm(constraint.Term{CID: c.QO, VID: c.XC})

	// qL⋅xa + qR⋅xb + qO⋅xc + qM⋅(xaxb) + qC == 0
	var t fr.Element
	t.Mul(&m0, &m1).Add(&t, &l).Add(&t, &r).Add(&t, &o).Add(&t, &solver.Coefficients[c.QC])
	if !t.IsZero() {
		return fmt.Errorf("qL⋅xa + qR⋅xb + qO⋅xc + qM⋅(xaxb) + qC != 0 → %s + %s + %s + (%s × %s) + %s != 0",
			l.String(),
			r.String(),
			o.String(),
			m0.String(),
			m1.String(),
			solver.Coefficients[c.QC].String(),
		)
	}
	return nil

}

// findUnsolvedWireSCS check the constraint for unsolved wire.
// If xA is unsolved returns 0
// If xB is unsolved returns 1
// If xC is unsolved returns 2
// If all wires are solved, returns -1
func (solver *solver) findUnsolvedWireSCS(c *constraint.SparseR1C) int {
	lID, rID, oID := c.XA, c.XB, c.XC

	if (c.QL != 0 || c.QM != 0) && !solver.solved[lID] {
		return 0
	}

	if (c.QR != 0 || c.QM != 0) && !solver.solved[rID] {
		return 1
	}

	if (c.QO != 0) && !solver.solved[oID] {
		return 2
	}
	return -1
}

// solveSparseR1C solve any unsolved wire in given constraint and update the solver
// a SparseR1C may have up to one unsolved wire
// if it doesn't, then this function returns and does nothing
func (solver *solver) solveSparseR1C(c *constraint.SparseR1C) error {

	if c.Commitment == constraint.COMMITTED { // a constraint of the form f_L - PI_2 = 0 or f_L = Comm.
		return nil // these are there for enforcing the correctness of the commitment and can be skipped in solving time
	}

	lro := solver.findUnsolvedWireSCS(c)
	if lro == -1 {
		// no unsolved wire
		// can happen if the constraint contained only hint wires or if it's an assertion.
		return nil
	}
	if lro == 1 { // we solve for R: u1L+u2R+u3LR+u4O+k=0 => R(u2+u3L)+u1L+u4O+k = 0
		if !solver.solved[c.XA] {
			panic("L wire should be instantiated when we solve R")
		}
		var u1, u2, u3, den, num, v1, v2 fr.Element
		u3.Set(&solver.Coefficients[c.QM])
		u1.Set(&solver.Coefficients[c.QL])
		u2.Set(&solver.Coefficients[c.QR])
		den.Mul(&u3, &solver.values[c.XA]).Add(&den, &u2)

		v1 = solver.computeTerm(constraint.Term{CID: c.QL, VID: c.XA})
		v2 = solver.computeTerm(constraint.Term{CID: c.QO, VID: c.XC})
		num.Add(&v1, &v2).Add(&num, &solver.Coefficients[c.QC])

		// TODO find a way to do lazy div (/ batch inversion)
		num.Div(&num, &den).Neg(&num)
		solver.set(int(c.XB), num) // TODO @gbotrel unused path?
		return nil
	}

	if lro == 0 { // we solve for L: u1L+u2R+u3LR+u4O+k=0 => L(u1+u3R)+u2R+u4O+k = 0
		if !solver.solved[c.XB] {
			panic("R wire should be instantiated when we solve L")
		}
		var u1, u2, u3, den, num, v1, v2 fr.Element
		u3.Set(&solver.Coefficients[c.QM])
		u1.Set(&solver.Coefficients[c.QL])
		u2.Set(&solver.Coefficients[c.QR])
		den.Mul(&u3, &solver.values[c.XB]).Add(&den, &u1)

		v1 = solver.computeTerm(constraint.Term{CID: c.QR, VID: c.XB})
		v2 = solver.computeTerm(constraint.Term{CID: c.QO, VID: c.XC})
		num.Add(&v1, &v2).Add(&num, &solver.Coefficients[c.QC])

		// TODO find a way to do lazy div (/ batch inversion)
		num.Div(&num, &den).Neg(&num)
		solver.set(int(c.XA), num)
		return nil

	}
	// O we solve for O
	var o fr.Element
	cID, vID := c.QO, c.XC

	l := solver.computeTerm(constraint.Term{CID: c.QL, VID: c.XA})
	r := solver.computeTerm(constraint.Term{CID: c.QR, VID: c.XB})
	m0 := solver.computeTerm(constraint.Term{CID: c.QM, VID: c.XA})
	m1 := solver.values[c.XB]

	// o = - ((m0 * m1) + l + r + c.QC) / c.O
	o.Mul(&m0, &m1).Add(&o, &l).Add(&o, &r).Add(&o, &solver.Coefficients[c.QC])

	// TODO @gbotrel seems it's the only place we use coefficientsNegInv
	// and I suspect most of the time c.QO == 1 or -1.
	o.Mul(&o, &solver.coefficientsNegInv[cID])

	solver.set(int(vID), o)
	return nil
}
