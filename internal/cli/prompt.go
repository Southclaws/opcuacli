package cli

import (
	"errors"
	"fmt"
	"os"

	"charm.land/huh/v2"
	"github.com/charmbracelet/x/term"
)

// ErrNotInteractive reports that a prompt was needed but there is no terminal to
// show it on. It is a distinct error because the remedy is a flag, not a retry.
var ErrNotInteractive = errors.New("no terminal to prompt on")

// interactive reports whether prompts can be shown. A piped stdin or a
// redirected stdout means a script is driving, and a script must not be asked a
// question it cannot answer.
func interactive() bool {
	return term.IsTerminal(os.Stdin.Fd()) && term.IsTerminal(os.Stdout.Fd())
}

// promptConfirm asks a yes/no question.
func promptConfirm(question string) (bool, error) {
	if !interactive() {
		return false, ErrNotInteractive
	}

	answer := false
	field := huh.NewConfirm().
		Title(question).
		Affirmative("Yes").
		Negative("No").
		Value(&answer)

	if err := huh.Run(field); err != nil {
		if errors.Is(err, huh.ErrUserAborted) {
			return false, nil
		}
		return false, err
	}
	return answer, nil
}

// profileForm is the answers collected by the interactive profile wizard.
type profileForm struct {
	Name        string
	Endpoint    string
	Policy      string
	Mode        string
	AuthMode    string
	Username    string
	Password    string
	MakeDefault bool
}

// runProfileForm asks for the settings of a new profile.
//
// The security and authentication choices are offered from what the server
// actually advertises where possible, since the useful answer is always one of
// those and guessing is how a first connection fails.
func runProfileForm(form *profileForm, policies, modes, tokens []string) error {
	if !interactive() {
		return ErrNotInteractive
	}

	if len(policies) == 0 {
		policies = []string{"auto", "None", "Basic256Sha256", "Aes128Sha256RsaOaep", "Aes256Sha256RsaPss"}
	}
	if len(modes) == 0 {
		modes = []string{"auto", "None", "Sign", "SignAndEncrypt"}
	}
	if len(tokens) == 0 {
		tokens = []string{"auto", "anonymous", "username", "certificate"}
	}

	groups := []*huh.Group{
		huh.NewGroup(
			huh.NewInput().
				Title("Profile name").
				Description("The name you will pass to --profile.").
				Value(&form.Name).
				Validate(huh.ValidateNotEmpty()),
			huh.NewInput().
				Title("Endpoint URL").
				Description("For example opc.tcp://localhost:4840.").
				Value(&form.Endpoint).
				Validate(huh.ValidateNotEmpty()),
		),
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Security policy").
				Description("auto takes the strongest the server offers.").
				Options(huh.NewOptions(policies...)...).
				Value(&form.Policy),
			huh.NewSelect[string]().
				Title("Message security mode").
				Options(huh.NewOptions(modes...)...).
				Value(&form.Mode),
			huh.NewSelect[string]().
				Title("Authentication").
				Options(huh.NewOptions(tokens...)...).
				Value(&form.AuthMode),
		),
		huh.NewGroup(
			huh.NewInput().
				Title("User name").
				Description("Leave empty for an anonymous session.").
				Value(&form.Username),
			huh.NewInput().
				Title("Password").
				Description("Stored in plain text; leave empty to use OPCUA_PASSWORD instead.").
				EchoMode(huh.EchoModePassword).
				Value(&form.Password),
		).WithHideFunc(func() bool {
			return form.AuthMode != "username"
		}),
		huh.NewGroup(
			huh.NewConfirm().
				Title("Make this the default profile?").
				Value(&form.MakeDefault),
		),
	}

	if err := huh.NewForm(groups...).Run(); err != nil {
		if errors.Is(err, huh.ErrUserAborted) {
			return fmt.Errorf("cancelled")
		}
		return err
	}
	return nil
}
