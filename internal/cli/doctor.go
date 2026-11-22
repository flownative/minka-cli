package cli

import (
	"errors"
	"fmt"
	"io"
	"path/filepath"

	"github.com/flownative/minka-cli/internal/config"
	"github.com/spf13/cobra"
)

const (
	checkOK   = "ok"
	checkWarn = "warn"
	checkFail = "fail"
	checkSkip = "skip" // not looked at, because a check before it failed
)

type doctorCheck struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Detail string `json:"detail"`
	Hint   string `json:"hint,omitempty"`
}

// newDoctorCmd looks at everything a command needs to work and says what is
// wrong with it. It is a report: it exits 0 whatever it finds, and
// data.healthy tells whether a check failed.
func (a *app) newDoctorCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Check the config file, the sign-in, the host, the skill and the version",
		Long: `Checks what a command needs to work: the config file and its permissions,
the host and the token, whether the host answers and accepts the token,
whether the installed skill is the one this minka carries, and how this minka
stands against the latest and the oldest version Minka names. It names the
profile a command uses and where that choice came from, and the default
profile where it is another one.

It is a report and exits 0 whatever it finds. With --json, data.healthy is
false when a check failed; every check carries status (ok, warn, fail, or
skip where a check before it failed), detail and, where something can be
done, hint. data.host is the host of the command, data.defaultHost the host of
the default profile, data.profile the profile of the command and
data.defaultProfile the default profile; each is null where there is none.`,
		Args: requireArgs(),
		RunE: func(*cobra.Command, []string) error {
			checks, host, profile := a.doctorChecks()
			defaultProfile, defaultHost, _ := a.defaultToReport()

			healthy := true
			problems := 0
			for _, check := range checks {
				if check.Status == checkFail {
					healthy = false
				}
				if check.Status == checkWarn || check.Status == checkFail {
					problems++
				}
			}
			summary := "Everything is fine"
			if problems > 0 {
				summary = plural(problems, "check needs", "checks need") + " attention"
			}

			a.skillIsTheTopic = true
			return a.ok(success{
				data: map[string]any{
					"healthy": healthy, "version": version, "checks": checks,
					"host": nullable(host), "defaultHost": nullable(defaultHost),
					"profile": nullable(profile), "defaultProfile": nullable(defaultProfile),
				},
				summary: summary,
				text: func(w io.Writer) {
					for _, check := range checks {
						fmt.Fprintf(w, "%-4s  %-8s %s\n", check.Status, check.Name, check.Detail)
						if check.Hint != "" {
							fmt.Fprintf(w, "               %s\n", check.Hint)
						}
					}
				},
			})
		},
	}
}

// doctorChecks runs the checks and names the host and the profile of the
// command: the ones the session reached, else the ones the choice names, else
// none.
func (a *app) doctorChecks() ([]doctorCheck, string, string) {
	checks := []doctorCheck{{Name: "version", Status: checkOK, Detail: "minka " + version}}

	configCheck := doctorCheck{Name: "config", Status: checkOK}
	if path, err := config.Path(a.getenv); err != nil {
		configCheck.Status, configCheck.Detail = checkFail, err.Error()
	} else if _, _, err := a.loadConfig(); err != nil {
		configCheck.Status, configCheck.Detail, configCheck.Hint = checkFail, err.Error(), hintOf(err)
	} else {
		configCheck.Detail = path
	}
	checks = append(checks, configCheck)

	signIn := doctorCheck{Name: "sign-in", Status: checkOK}
	host := doctorCheck{Name: "host", Status: checkOK}
	c, path, file, err := a.chooseAccount(true)
	var s *session
	if err == nil {
		s, err = a.sessionOf(c, path, file)
	}
	sessionHost, sessionProfile := "", ""
	if err != nil {
		signIn.Status, signIn.Detail, signIn.Hint = checkFail, err.Error(), hintOf(err)
		var cliErr *Error
		switch {
		case errors.As(err, &cliErr) && cliErr.Code == CodeAuth:
			// An agent cannot run setup, so the hint it can follow comes first.
			signIn.Hint += " Or, on a terminal: minka setup"
		case configCheck.Status == checkFail && errors.As(err, &cliErr) && cliErr.Code == CodeConfig:
			signIn.Status, signIn.Detail, signIn.Hint = checkSkip, "not checked, see config", ""
		}
		host.Status, host.Detail = checkSkip, "not checked without a sign-in"
		// The account the command was given is still the one it would reach.
		sessionHost, sessionProfile = c.host, c.profile
	} else {
		sessionHost, sessionProfile = s.host, s.profile
		signIn.Detail = "a token for " + s.host + " from the config file"
		if s.tokenSource == tokenFromEnvironment {
			signIn.Detail = "a token for " + s.host + " from $MINKA_TOKEN"
		}
		defaultProfile, _, problem := a.defaultToReport()
		signIn.Detail += "; " + accountInWords(c, defaultProfile, problem)
		if me, err := s.client.Me(a.ctx); err != nil {
			e := s.fail(err)
			host.Status, host.Detail, host.Hint = checkFail, e.Message, e.Hint
		} else {
			host.Detail = s.host + " answers, signed in as " + me.Value.Name
			if me.Value.Principal != nil {
				host.Detail += ", acting for " + me.Value.Principal.Name
			}
		}
	}
	checks = append(checks, signIn, host)

	return append(checks, a.doctorSkillCheck(), a.doctorUpdateCheck()), sessionHost, sessionProfile
}

func hintOf(err error) string {
	var cliErr *Error
	if errors.As(err, &cliErr) {
		return cliErr.Hint
	}
	return ""
}

func (a *app) doctorSkillCheck() doctorCheck {
	check := doctorCheck{Name: "skill", Status: checkOK}
	skillsDir, err := a.skillsDir("")
	if err != nil {
		check.Status, check.Detail = checkWarn, err.Error()
		return check
	}
	path := filepath.Join(skillsDir, skillName, skillFile)
	if link := firstSymlink(skillsDir, filepath.Join(skillsDir, skillName)); link != "" {
		check.Detail = link + " is a symbolic link and kept current by whoever owns it"
		return check
	}

	switch skillState(path) {
	case skillCurrent:
		check.Detail = path + " is the skill this minka carries"
	case skillDiffers:
		check.Status, check.Detail, check.Hint = checkWarn, path+" differs from the skill this minka carries", "Run: minka skill install"
	default:
		check.Status, check.Detail, check.Hint = checkWarn, "no skill at "+path, "Run: minka skill install"
	}
	return check
}
