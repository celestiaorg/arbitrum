package protocol

import (
	"io"

	"github.com/offchainlabs/arb-avm/value"
	"github.com/offchainlabs/arb-avm/vm"
)

type AssertionDefender struct {
	assertion    *Assertion
	precondition *Precondition
	beforeInbox  value.Value
	initState    *vm.Machine
}

func NewAssertionDefender(assertion *Assertion, precondition *Precondition, beforeInbox value.Value, initState *vm.Machine) AssertionDefender {
	return AssertionDefender{assertion, precondition, beforeInbox, initState.Clone()}
}

func (ad AssertionDefender) NumSteps() uint32 {
	return ad.assertion.NumSteps
}

func (ad AssertionDefender) GetAssertion() *Assertion {
	return ad.assertion
}

func (ad AssertionDefender) GetPrecondition() *Precondition {
	return ad.precondition
}

func (ad AssertionDefender) GetInbox() value.Value {
	return ad.beforeInbox
}

func (ad AssertionDefender) GetMachineState() *vm.Machine {
	return ad.initState
}

func (ad AssertionDefender) NBisect(slices uint32) []AssertionDefender {
	nsteps := ad.NumSteps()
	if nsteps < slices {
		slices = nsteps
	}
	sliceSize := nsteps / slices
	defenders := make([]AssertionDefender, 0, slices)
	machine := ad.initState.Clone()

	precondition := ad.precondition
	for i := uint32(0); i < slices; i++ {
		runState := machine.Clone()
		ctx1 := NewMachineAssertionContext(runState, precondition.BeforeBalance, precondition.TimeBounds, ad.beforeInbox)
		stepCount := sliceSize
		if i < nsteps%slices {
			stepCount++
		}
		runState.Run(int32(stepCount))
		defender := ctx1.Finalize(machine)
		defenders = append(defenders, defender)
		precondition = defender.GetAssertion().Stub().GeneratePostcondition(precondition)
		machine = runState
	}
	return defenders
}

func (ad AssertionDefender) SolidityOneStepProof(proofWr io.Writer) error {
	return ad.initState.MarshalForProof(proofWr)
}
