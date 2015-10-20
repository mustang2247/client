package engine

import (
	"time"

	"github.com/keybase/client/go/kex2"
	"github.com/keybase/client/go/libkb"
	keybase1 "github.com/keybase/client/go/protocol"
	rpc "github.com/keybase/go-framed-msgpack-rpc"
	jsonw "github.com/keybase/go-jsonw"
	"golang.org/x/net/context"
)

// Kex2Provisioner is an engine.
type Kex2Provisioner struct {
	libkb.Contextified
	deviceID   keybase1.DeviceID
	secret     kex2.Secret
	secretCh   chan kex2.Secret
	me         *libkb.User
	signingKey libkb.GenericKey
	pps        *libkb.PassphraseStream
	ctx        *Context
}

// Kex2Provisioner implements kex2.Provisioner interface.
var _ kex2.Provisioner = (*Kex2Provisioner)(nil)

// NewKex2Provisioner creates a Kex2Provisioner engine.
// XXX should be able to remove deviceID parameter.  Provisioner
// is signed in.  Can get device id from env.
func NewKex2Provisioner(g *libkb.GlobalContext, deviceID keybase1.DeviceID, secret kex2.Secret) *Kex2Provisioner {
	return &Kex2Provisioner{
		Contextified: libkb.NewContextified(g),
		deviceID:     deviceID,
		secret:       secret,
		secretCh:     make(chan kex2.Secret),
	}
}

// Name is the unique engine name.
func (e *Kex2Provisioner) Name() string {
	return "Kex2Provisioner"
}

// GetPrereqs returns the engine prereqs.
func (e *Kex2Provisioner) Prereqs() Prereqs {
	return Prereqs{Device: true}
}

// RequiredUIs returns the required UIs.
func (e *Kex2Provisioner) RequiredUIs() []libkb.UIKind {
	return []libkb.UIKind{
		libkb.SecretUIKind,
		libkb.ProvisionUIKind,
	}
}

// SubConsumers returns the other UI consumers for this engine.
func (e *Kex2Provisioner) SubConsumers() []libkb.UIConsumer {
	return nil
}

// Run starts the provisioner engine.
func (e *Kex2Provisioner) Run(ctx *Context) (err error) {
	e.G().Log.Debug("+ Kex2Provisioner.Run()")
	defer func() { e.G().Log.Debug("- Kex2Provisioner.Run() -> %s", libkb.ErrToOk(err)) }()

	// before starting provisioning, need to load some information:

	// load self:
	e.me, err = libkb.LoadMe(libkb.NewLoadUserArg(e.G()))
	if err != nil {
		return err
	}

	// get signing key (including secret key):
	arg := libkb.SecretKeyArg{
		Me:      e.me,
		KeyType: libkb.DeviceSigningKeyType,
	}
	e.signingKey, err = e.G().Keyrings.GetSecretKeyWithPrompt(ctx.LoginContext, arg, ctx.SecretUI, "new device install")
	if err != nil {
		return err
	}

	// get current passphrase stream:
	e.pps, err = e.G().LoginState().GetPassphraseStream(ctx.SecretUI)
	if err != nil {
		return err
	}

	// ctx needed by some kex2 functions
	e.ctx = ctx

	// all set:  start provisioner
	karg := kex2.KexBaseArg{
		Ctx:           context.TODO(),
		Mr:            libkb.NewKexRouter(e.G()),
		DeviceID:      e.deviceID,
		Secret:        e.secret,
		SecretChannel: e.secretCh,
		Timeout:       5 * time.Minute,
	}
	parg := kex2.ProvisionerArg{
		KexBaseArg:  karg,
		Provisioner: e,
	}
	err = kex2.RunProvisioner(parg)

	return err
}

// AddSecret inserts a received secret into the provisioner's
// secret channel.
func (e *Kex2Provisioner) AddSecret(s kex2.Secret) {
	e.secretCh <- s
}

// GetLogFactory implements GetLogFactory in kex2.Provisioner.
func (e *Kex2Provisioner) GetLogFactory() rpc.LogFactory {
	return rpc.NewSimpleLogFactory(e.G().Log, nil)
}

// GetHelloArg implements GetHelloArg in kex2.Provisioner.
func (e *Kex2Provisioner) GetHelloArg() (arg keybase1.HelloArg, err error) {
	e.G().Log.Debug("+ GetHelloArg()")
	defer func() { e.G().Log.Debug("- GetHelloArg() -> %s", libkb.ErrToOk(err)) }()

	e.ctx.ProvisionUI.DisplaySecretExchanged(context.TODO(), 0)

	// get a session token that device Y can use
	token, csrf, err := e.sessionForY()
	if err != nil {
		return arg, err
	}

	// generate a skeleton key proof
	sigBody, err := e.skeletonProof()
	if err != nil {
		return arg, err
	}

	// return the HelloArg
	arg = keybase1.HelloArg{
		Uid:     e.me.GetUID(),
		Pps:     e.pps.Export(),
		Token:   keybase1.SessionToken(token),
		Csrf:    keybase1.CsrfToken(csrf),
		SigBody: sigBody,
	}
	return arg, nil
}

// CounterSign implements CounterSign in kex2.Provisioner.
func (e *Kex2Provisioner) CounterSign(input keybase1.HelloRes) (sig []byte, err error) {
	e.G().Log.Debug("+ CounterSign()")
	defer func() { e.G().Log.Debug("- CounterSign() -> %s", libkb.ErrToOk(err)) }()

	jw, err := jsonw.Unmarshal([]byte(input))
	if err != nil {
		return nil, err
	}

	// check the reverse signature
	if err = e.checkReverseSig(jw); err != nil {
		e.G().Log.Debug("provisioner failed to verify reverse sig: %s", err)
		return nil, err
	}
	e.G().Log.Debug("provisioner verified reverse sig")

	// sign the whole thing with provisioner's signing key
	s, _, _, err := libkb.SignJSON(jw, e.signingKey)
	if err != nil {
		return nil, err
	}

	return []byte(s), nil
}

// sessionForY gets session tokens that Y can use to interact with
// API server.
func (e *Kex2Provisioner) sessionForY() (token, csrf string, err error) {
	resp, err := e.G().API.Post(libkb.APIArg{
		Endpoint:     "new_session",
		NeedSession:  true,
		Contextified: libkb.NewContextified(e.G()),
	})
	if err != nil {
		return "", "", err
	}
	token, err = resp.Body.AtKey("session").GetString()
	if err != nil {
		return "", "", err
	}
	csrf, err = resp.Body.AtKey("csrf_token").GetString()
	if err != nil {
		return "", "", err
	}

	return token, csrf, nil
}

// skeletonProof generates a partial key proof structure that
// device Y can fill in.
func (e *Kex2Provisioner) skeletonProof() (string, error) {
	delg := libkb.Delegator{
		ExistingKey:    e.signingKey,
		Me:             e.me,
		DelegationType: libkb.SibkeyType,
		Expire:         libkb.NaclEdDSAExpireIn,
		Contextified:   libkb.NewContextified(e.G()),
	}

	jw, err := libkb.KeyProof(delg)
	if err != nil {
		return "", err
	}
	body, err := jw.Marshal()
	if err != nil {
		return "", err
	}
	return string(body), nil
}

// checkReverseSig verifies that the reverse sig in jw is valid
// and matches jw.
func (e *Kex2Provisioner) checkReverseSig(jw *jsonw.Wrapper) error {
	kid, err := jw.AtPath("body.sibkey.kid").GetString()
	if err != nil {
		return err
	}

	keypair, err := libkb.ImportKeypairFromKID(keybase1.KIDFromString(kid))
	if err != nil {
		return err
	}

	revsig, err := jw.AtPath("body.sibkey.reverse_sig").GetString()
	if err != nil {
		return err
	}

	// set reverse_sig to nil to verify it:
	jw.SetValueAtPath("body.sibkey.reverse_sig", jsonw.NewNil())
	msg, err := jw.Marshal()
	if err != nil {
		return err
	}
	_, err = keypair.VerifyString(revsig, msg)
	if err != nil {
		return err
	}

	// put reverse_sig back in
	jw.SetValueAtPath("body.sibkey.reverse_sig", jsonw.NewString(revsig))

	return nil
}
