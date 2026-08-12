package cli

import (
	"context"
	"image/color"
	"os"
	"sort"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/carapace-sh/carapace"
	"github.com/charmbracelet/fang"
	"github.com/charmbracelet/x/exp/charmtone"

	"github.com/Southclaws/opcuacli/internal/config"
	"github.com/Southclaws/opcuacli/internal/conn"
	"github.com/Southclaws/opcuacli/internal/opc"
)

// The completion values come from the same tables the parsers use, so a shell can
// never offer a value a command would reject.
func attributeNames() []string     { return opc.AttributeNames() }
func referenceTypeNames() []string { return opc.ReferenceTypeNames() }
func nodeClassNames() []string     { return opc.NodeClassNames() }
func writableTypeNames() []string  { return opc.WritableTypeNames() }

// actionPolicies offers the security policies this client can negotiate.
func actionPolicies() carapace.Action {
	return carapace.ActionValues(
		"auto",
		"None",
		"Basic128Rsa15",
		"Basic256",
		"Basic256Sha256",
		"Aes128_Sha256_RsaOaep",
		"Aes256_Sha256_RsaPss",
	)
}

// actionProfiles offers the profiles in the configuration file, describing each
// with its endpoint so a name is recognisable.
func actionProfiles() carapace.Action {
	return carapace.ActionCallback(func(carapace.Context) carapace.Action {
		file, err := config.Load("")
		if err != nil {
			return carapace.ActionMessage("cannot read the configuration: %v", err)
		}

		values := make([]string, 0, len(file.Profiles)*2)
		for _, name := range file.Names() {
			values = append(values, name, file.Profiles[name].Endpoint())
		}
		if len(values) == 0 {
			return carapace.ActionMessage("no profiles configured; run opcua config init")
		}
		return carapace.ActionValuesDescribed(values...)
	})
}

// actionSettings offers the dotted keys `config set` understands.
func actionSettings() carapace.Action {
	return carapace.ActionValues(config.SettableKeys...)
}

// actionEndpoints offers the endpoints already configured, since an endpoint URL
// is otherwise unguessable.
func actionEndpoints() carapace.Action {
	return carapace.ActionCallback(func(carapace.Context) carapace.Action {
		file, err := config.Load("")
		if err != nil {
			return carapace.ActionValues(config.DefaultEndpoint)
		}

		seen := map[string]bool{}
		values := []string{}
		for _, name := range file.Names() {
			for _, endpoint := range file.Profiles[name].Endpoints {
				if seen[endpoint] {
					continue
				}
				seen[endpoint] = true
				values = append(values, endpoint, name)
			}
		}
		if len(values) == 0 {
			return carapace.ActionValues(config.DefaultEndpoint)
		}
		return carapace.ActionValuesDescribed(values...)
	})
}

// actionNodes completes node ids by browsing the active profile's server.
//
// Completing against the live address space is the difference between typing a
// node id from memory and picking one; the well-known entry points are offered
// too, so it is still useful when no server can be reached.
func actionNodes() carapace.Action {
	return carapace.ActionCallback(func(c carapace.Context) carapace.Action {
		wellKnown := wellKnownValues()

		nodes, err := browseForCompletion(c)
		if err != nil {
			// A completion must never block or complain: an unreachable server
			// just means the static entry points are all that can be offered.
			return carapace.ActionValuesDescribed(wellKnown...)
		}
		return carapace.ActionValuesDescribed(append(nodes, wellKnown...)...)
	})
}

// wellKnownValues are the standard entry points, as value/description pairs.
func wellKnownValues() []string {
	names := make([]string, 0, len(opc.WellKnownNodes))
	for name := range opc.WellKnownNodes {
		names = append(names, name)
	}
	sort.Strings(names)

	values := make([]string, 0, len(names)*2)
	for _, name := range names {
		values = append(values, name, "standard entry point")
	}
	return values
}

// browseForCompletion reads one level of the address space, bounded hard in time
// so that pressing tab never leaves a shell waiting.
func browseForCompletion(c carapace.Context) ([]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	settings := &conn.Settings{
		Endpoints:      []string{completionEndpoint(c)},
		Policy:         "auto",
		Mode:           "auto",
		AuthMode:       "auto",
		RequestTimeout: 2 * time.Second,
		DialTimeout:    2 * time.Second,
		SessionTimeout: time.Minute,
		Username:       c.Getenv("OPCUA_USERNAME"),
		Password:       c.Getenv("OPCUA_PASSWORD"),
	}

	client, err := conn.Dial(ctx, settings, nil)
	if err != nil {
		return nil, err
	}
	defer client.CloseQuietly(ctx)

	objects, err := opc.ParseNodeID("Objects")
	if err != nil {
		return nil, err
	}

	found, err := opc.Browse(ctx, client, opc.BrowseOptions{
		Root:            objects,
		Depth:           2,
		IncludeSubtypes: true,
		Limit:           250,
	})
	if err != nil {
		return nil, err
	}

	values := make([]string, 0, len(found)*2)
	for _, node := range found {
		description := node.BrowseName
		if node.Path != nil {
			description = *node.Path
		}
		values = append(values, node.NodeID, description)
	}
	return values, nil
}

// completionEndpoint resolves which server to browse for completions: the
// environment first, then the active profile.
func completionEndpoint(c carapace.Context) string {
	if endpoint := c.Getenv("OPCUA_ENDPOINT"); endpoint != "" {
		return endpoint
	}

	file, err := config.Load(os.Getenv("OPCUA_CONFIG"))
	if err != nil {
		return config.DefaultEndpoint
	}
	_, profile, err := file.Resolve(c.Getenv("OPCUA_PROFILE"))
	if err != nil {
		return config.DefaultEndpoint
	}
	return profile.Endpoint()
}

// colorScheme themes fang's help, error and version output with the same
// CharmTone palette the rest of the output uses, so help and results look like
// one program.
func colorScheme(light lipgloss.LightDarkFunc) fang.ColorScheme {
	hex := func(key charmtone.Key) color.Color { return lipgloss.Color(key.Hex()) }

	base := fang.DefaultColorScheme(light)
	base.Title = hex(charmtone.Charple)
	base.Command = hex(charmtone.Malibu)
	base.Flag = hex(charmtone.Guac)
	base.Argument = light(hex(charmtone.Pepper), hex(charmtone.Salt))
	base.Program = hex(charmtone.Charple)
	base.DimmedArgument = hex(charmtone.Squid)
	base.Description = light(hex(charmtone.Charcoal), hex(charmtone.Smoke))
	base.ErrorHeader = [2]color.Color{hex(charmtone.Salt), hex(charmtone.Sriracha)}
	return base
}
