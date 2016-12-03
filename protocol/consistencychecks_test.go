package protocol

import (
	"bytes"
	"testing"

	m "github.com/coniks-sys/coniks-go/merkletree"
)

var (
	alice = "alice"
	bob   = "bob"
	key   = []byte("key")
)

func registerAndVerify(d *ConiksDirectory, cc *ConsistencyChecks,
	name string, key []byte) (error, error) {
	request := &RegistrationRequest{
		Username: name,
		Key:      key,
	}
	res, err := d.Register(request)
	return err, cc.HandleResponse(RegistrationType, res, name, key)
}

func lookupAndVerify(d *ConiksDirectory, cc *ConsistencyChecks,
	name string, key []byte) (error, error) {
	request := &KeyLookupRequest{
		Username: name,
	}
	res, err := d.KeyLookup(request)
	return err, cc.HandleResponse(KeyLookupType, res, name, key)
}

func monitorAndVerify(d *ConiksDirectory, cc *ConsistencyChecks,
	name string, key []byte, startEp, endEp uint64) error {
	request := &MonitoringRequest{
		Username:   name,
		StartEpoch: startEp,
		EndEpoch:   endEp,
	}
	res, _ := d.Monitor(request)
	return cc.HandleResponse(MonitoringType, res, name, key)
}

func TestVerifyWithError(t *testing.T) {
	d, pk := NewTestDirectory(t, true)

	// modify the pinning STR so that the consistency check should fail.
	str := *(d.LatestSTR())
	str.Signature = append([]byte{}, str.Signature...)
	str.Signature[0]++

	cc := NewCC(d.LatestSTR(), &str, true, pk)

	if e1, e2 := registerAndVerify(d, cc, alice, key); e1 != ReqSuccess || e2 != CheckBadSTR {
		t.Error("Expect", ReqSuccess, "got", e1)
		t.Error("Expect", CheckBadSTR, "got", e2)
	}
}

func TestMalformedClientMessage(t *testing.T) {
	d, pk := NewTestDirectory(t, true)
	cc := NewCC(d.LatestSTR(), d.LatestSTR(), true, pk)

	request := &RegistrationRequest{
		Username: "", // invalid username
		Key:      key,
	}
	res, _ := d.Register(request)
	if err := cc.HandleResponse(RegistrationType, res, "", key); err != ErrMalformedClientMessage {
		t.Error("Unexpected verification result")
	}
}

func TestMalformedDirectoryMessage(t *testing.T) {
	d, pk := NewTestDirectory(t, true)
	cc := NewCC(d.LatestSTR(), d.LatestSTR(), true, pk)

	request := &RegistrationRequest{
		Username: "alice",
		Key:      key,
	}
	res, _ := d.Register(request)
	// modify response message
	res.DirectoryResponse.(*DirectoryProof).STR = nil
	if err := cc.HandleResponse(RegistrationType, res, "alice", key); err != ErrMalformedDirectoryMessage {
		t.Error("Unexpected verification result")
	}
}

func TestVerifyRegistrationResponseWithTB(t *testing.T) {
	d, pk := NewTestDirectory(t, true)

	cc := NewCC(d.LatestSTR(), d.LatestSTR(), true, pk)

	if e1, e2 := registerAndVerify(d, cc, alice, key); e1 != ReqSuccess || e2 != CheckPassed {
		t.Error(e1)
		t.Error(e2)
	}

	if len(cc.TBs) != 1 {
		t.Fatal("Expect the directory to return a signed promise")
	}

	// test error name existed
	if e1, e2 := registerAndVerify(d, cc, alice, key); e1 != ReqNameExisted || e2 != CheckPassed {
		t.Error(e1)
		t.Error(e2)
	}

	// test error name existed with different key
	if e1, e2 := registerAndVerify(d, cc, alice, []byte{1, 2, 3}); e1 != ReqNameExisted || e2 != CheckBindingsDiffer {
		t.Error(e1)
		t.Error(e2)
	}
	if len(cc.TBs) != 1 {
		t.Fatal("Expect the directory to return a signed promise")
	}

	// re-register in a different epoch
	// Since the fulfilled promise verification would be perform
	// when the client is monitoring, we do _not_ expect a TB's verification here.
	d.Update()

	if e1, e2 := registerAndVerify(d, cc, alice, key); e1 != ReqNameExisted || e2 != CheckPassed {
		t.Error(e1)
		t.Error(e2)
	}
	if e1, e2 := registerAndVerify(d, cc, alice, []byte{1, 2, 3}); e1 != ReqNameExisted || e2 != CheckBindingsDiffer {
		t.Error(e1)
		t.Error(e2)
	}
}

func TestVerifyFullfilledPromise(t *testing.T) {
	d, pk := NewTestDirectory(t, true)

	cc := NewCC(d.LatestSTR(), d.LatestSTR(), true, pk)

	if e1, e2 := registerAndVerify(d, cc, alice, key); e1 != ReqSuccess || e2 != CheckPassed {
		t.Error(e1)
		t.Error(e2)
	}
	if e1, e2 := registerAndVerify(d, cc, bob, key); e1 != ReqSuccess || e2 != CheckPassed {
		t.Error(e1)
		t.Error(e2)
	}

	if len(cc.TBs) != 2 {
		t.Fatal("Expect the directory to return signed promises")
	}

	d.Update()

	for i := 0; i < 2; i++ {
		if e1, e2 := lookupAndVerify(d, cc, alice, key); e1 != ReqSuccess || e2 != CheckPassed {
			t.Error(e1)
			t.Error(e2)
		}
	}

	// should contain the TBs of bob
	if len(cc.TBs) != 1 || cc.TBs[bob] == nil {
		t.Error("Expect the directory to insert the binding as promised")
	}

	if e1, e2 := lookupAndVerify(d, cc, bob, key); e1 != ReqSuccess || e2 != CheckPassed {
		t.Error(e1)
		t.Error(e2)
	}
	if len(cc.TBs) != 0 {
		t.Error("Expect the directory to insert the binding as promised")
	}
}

func TestVerifyKeyLookupResponseWithTB(t *testing.T) {
	d, pk := NewTestDirectory(t, true)

	cc := NewCC(d.LatestSTR(), d.LatestSTR(), true, pk)

	// do lookup first
	if e1, e2 := lookupAndVerify(d, cc, alice, key); e1 != ReqNameNotFound || e2 != CheckPassed {
		t.Error(e1)
		t.Error(e2)
	}

	// register
	if e1, e2 := registerAndVerify(d, cc, alice, key); e1 != ReqSuccess || e2 != CheckPassed {
		t.Error(e1)
		t.Error(e2)
	}

	// do lookup in the same epoch - TB TOFU
	// and get the key from the response. The key would be extracted from the TB
	request := &KeyLookupRequest{
		Username: alice,
	}
	res, err := d.KeyLookup(request)
	if err != ReqSuccess {
		t.Error("Expect", ReqSuccess, "got", err)
	}
	if err := cc.HandleResponse(KeyLookupType, res, alice, nil); err != CheckPassed {
		t.Error("Expect", CheckPassed, "got", err)
	}
	recvKey, e := res.GetKey()
	if e != nil && !bytes.Equal(recvKey, key) {
		t.Error("The directory has returned a wrong key.")
	}

	d.Update()

	// do lookup in the different epoch
	// this time, the key would be extracted from the AP.
	request = &KeyLookupRequest{
		Username: alice,
	}
	res, err = d.KeyLookup(request)
	if err != ReqSuccess {
		t.Error("Expect", ReqSuccess, "got", err)
	}
	if err := cc.HandleResponse(KeyLookupType, res, alice, nil); err != CheckPassed {
		t.Error("Expect", CheckPassed, "got", err)
	}
	recvKey, e = res.GetKey()
	if e != nil && !bytes.Equal(recvKey, key) {
		t.Error("The directory has returned a wrong key.")
	}

	// test error name not found
	if e1, e2 := lookupAndVerify(d, cc, bob, key); e1 != ReqNameNotFound || e2 != CheckPassed {
		t.Error(e1)
		t.Error(e2)
	}
}

func TestVerifyTimeSkew(t *testing.T) {
	d, pk := NewTestDirectory(t, true)
	cc := NewCC(d.LatestSTR(), d.LatestSTR(), true, pk)

	N := 5

	// verify prior history
	for i := 0; i < N; i++ {
		d.Update()
	}
	if err := monitorAndVerify(d, cc, alice, nil, cc.SavedSTR.Epoch, d.LatestSTR().Epoch); err != CheckPassed {
		t.Error(err)
	}

	// register
	if _, e2 := registerAndVerify(d, cc, alice, key); e2 != CheckPassed {
		t.Error("Cannot register new binding")
	}

	// monitor binding was inserted
	d.Update()
	// do a lookup before the clock tells us to do monitoring
	if e1, e2 := lookupAndVerify(d, cc, alice, key); e1 != ReqSuccess || e2 != CheckPassed {
		t.Error(e1)
		t.Error(e2)
	}
	if err := monitorAndVerify(d, cc, alice, key, cc.SavedSTR.Epoch, d.LatestSTR().Epoch); err != CheckPassed {
		t.Error(err)
	}
}

func TestVerifyMonitoring(t *testing.T) {
	d, pk := NewTestDirectory(t, true)
	cc := NewCC(d.LatestSTR(), d.LatestSTR(), true, pk)

	N := 5

	// verify prior history
	for i := 0; i < N; i++ {
		d.Update()
	}
	if err := monitorAndVerify(d, cc, alice, nil, 0, d.LatestSTR().Epoch); err != CheckPassed {
		t.Error(err)
	}

	// register
	if _, e2 := registerAndVerify(d, cc, alice, key); e2 != CheckPassed {
		t.Error("Cannot register new binding")
	}
	// or we can verify prior history after registering
	if err := monitorAndVerify(d, cc, alice, nil, 0, d.LatestSTR().Epoch); err != CheckPassed {
		t.Error(err)
	}

	// monitor binding was inserted
	d.Update()
	if err := monitorAndVerify(d, cc, alice, key, cc.SavedSTR.Epoch, d.LatestSTR().Epoch); err != CheckPassed {
		t.Error(err)
	}

	// monitor
	for i := 0; i < N; i++ {
		d.Update()
	}
	if err := monitorAndVerify(d, cc, alice, key, cc.SavedSTR.Epoch, d.LatestSTR().Epoch); err != CheckPassed {
		t.Error(err)
	}

	for i := 0; i < N; i++ {
		d.Update()
	}
	if err := monitorAndVerify(d, cc, alice, key, cc.SavedSTR.Epoch, d.LatestSTR().Epoch); err != CheckPassed {
		t.Error(err)
	}
}

// Expect the ConsistencyChecks to panic:
// - If: StartEpoch < SavedEpoch + 1
func TestVerifyMonitoringBadEpoch0(t *testing.T) {
	d, pk := NewTestDirectory(t, true)
	cc := NewCC(d.LatestSTR(), d.LatestSTR(), true, pk)

	N := 5

	for i := 0; i < N; i++ {
		d.Update()
	}
	if err := monitorAndVerify(d, cc, alice, nil, 0, d.LatestSTR().Epoch); err != CheckPassed {
		t.Error("Unexpected verification result")
	}

	defer func() {
		if recover() == nil {
			t.Fatal("Expect HandleResponse panic")
		}
	}()
	if err := monitorAndVerify(d, cc, alice, nil, cc.SavedSTR.Epoch-1, d.LatestSTR().Epoch); err != CheckPassed {
		t.Error(err)
	}
}

// Expect the ConsistencyChecks to panic:
// - If: StartEpoch > SavedEpoch + 1
func TestVerifyMonitoringBadEpoch1(t *testing.T) {
	d, pk := NewTestDirectory(t, true)
	cc := NewCC(d.LatestSTR(), d.LatestSTR(), true, pk)

	N := 5

	for i := 0; i < N; i++ {
		d.Update()
	}
	if err := monitorAndVerify(d, cc, alice, nil, 0, d.LatestSTR().Epoch); err != CheckPassed {
		t.Error("Unexpected verification result")
	}

	for i := 0; i < N; i++ {
		d.Update()
	}

	defer func() {
		if recover() == nil {
			t.Fatal("Expect HandleResponse panic")
		}
	}()
	if err := monitorAndVerify(d, cc, alice, nil, cc.SavedSTR.Epoch+2, d.LatestSTR().Epoch); err != CheckPassed {
		t.Error(err)
	}
}

func TestMalformedMonitoringResponse(t *testing.T) {
	d, pk := NewTestDirectory(t, true)
	cc := NewCC(d.LatestSTR(), d.LatestSTR(), true, pk)

	// len(AP) == 0
	malformedResponse := &Response{
		Error: ReqSuccess,
		DirectoryResponse: &DirectoryProofs{
			AP:  nil,
			STR: append([]*m.SignedTreeRoot{}, &m.SignedTreeRoot{}),
		},
	}
	if err := cc.HandleResponse(MonitoringType, malformedResponse, alice, key); err != ErrMalformedDirectoryMessage {
		t.Error(err)
	}

	// len(AP) != len(STR)
	str2 := append([]*m.SignedTreeRoot{}, &m.SignedTreeRoot{})
	str2 = append(str2, &m.SignedTreeRoot{})
	malformedResponse = &Response{
		Error: ReqSuccess,
		DirectoryResponse: &DirectoryProofs{
			AP:  append([]*m.AuthenticationPath{}, &m.AuthenticationPath{}),
			STR: str2,
		},
	}
	if err := cc.HandleResponse(MonitoringType, malformedResponse, alice, key); err != ErrMalformedDirectoryMessage {
		t.Error(err)
	}

	// len(STR) == 0
	malformedResponse = &Response{
		Error: ReqSuccess,
		DirectoryResponse: &DirectoryProofs{
			AP:  append([]*m.AuthenticationPath{}, &m.AuthenticationPath{}),
			STR: nil,
		},
	}
	if err := cc.HandleResponse(MonitoringType, malformedResponse, alice, key); err != ErrMalformedDirectoryMessage {
		t.Error(err)
	}

	// Error != ReqSuccess
	malformedResponse = &Response{
		Error: ReqNameNotFound,
		DirectoryResponse: &DirectoryProofs{
			AP:  append([]*m.AuthenticationPath{}, &m.AuthenticationPath{}),
			STR: nil,
		},
	}
	if err := cc.HandleResponse(MonitoringType, malformedResponse, alice, key); err != ErrMalformedDirectoryMessage {
		t.Error(err)
	}
}
