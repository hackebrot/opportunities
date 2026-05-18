package prompt

import "github.com/charmbracelet/huh"

// Huh is the production prompt.Interface implementation; it drives huh
// forms on the terminal. Not suitable for unit tests (requires a TTY);
// tests should install a deterministic stub via WithInterface.
type Huh struct{}

func (Huh) Select(title string, options []Option) (string, error) {
	huhOpts := make([]huh.Option[string], len(options))
	for i, o := range options {
		huhOpts[i] = huh.NewOption(o.Label, o.Key)
	}
	var chosen string
	form := huh.NewSelect[string]().
		Title(title).
		Options(huhOpts...).
		Value(&chosen)
	if err := form.Run(); err != nil {
		return "", err
	}
	return chosen, nil
}

func (Huh) Text(title string, validate func(string) error) (string, error) {
	var v string
	input := huh.NewInput().Title(title).Value(&v)
	if validate != nil {
		input = input.Validate(validate)
	}
	if err := input.Run(); err != nil {
		return "", err
	}
	return v, nil
}

func (Huh) Confirm(title string) (bool, error) {
	var v bool
	form := huh.NewConfirm().Title(title).Value(&v)
	if err := form.Run(); err != nil {
		return false, err
	}
	return v, nil
}
