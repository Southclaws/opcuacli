package cli

import (
	"context"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gopcua/opcua"
	"github.com/spf13/cobra"

	"github.com/Southclaws/opcuacli/internal/cligen"
	"github.com/Southclaws/opcuacli/internal/config"
	"github.com/Southclaws/opcuacli/internal/conn"
	"github.com/Southclaws/opcuacli/internal/pki"
	"github.com/Southclaws/opcuacli/internal/render"
)

// certGenerate writes a client certificate and key for secure channels.
func certGenerate(ctx context.Context, cmd *cobra.Command, commandIO cligen.IO, p cligen.CertGenerateParams) error {
	streams := newOutput(cmd, commandIO)

	settings, file, err := conn.FromCommand(cmd, commandIO.In)
	if err != nil {
		return err
	}

	directory := p.OutDir
	if directory == "" {
		directory = config.CertDir()
	}

	pair, err := pki.Generate(pki.Request{
		CommonName:     p.CommonName,
		ApplicationURI: settings.ApplicationURI,
		DNSNames:       p.Dns,
		IPAddresses:    p.Ip,
		Bits:           p.Bits,
		Validity:       time.Duration(p.Days) * 24 * time.Hour,
	})
	if err != nil {
		return err
	}
	if err := pair.Write(directory, p.Force); err != nil {
		return err
	}

	streams.Out.Okf("wrote %s and %s", pair.CertPath, pair.KeyPath)

	info, err := pki.Describe(pair.CertDER, pair.CertPath)
	if err != nil {
		return err
	}
	renderCertificate(streams.Out, info)

	if p.UpdateProfile {
		if settings.Profile == "" {
			streams.Err.Warnf("no profile is active, so nothing was updated; pass --cert %s and --key %s, or run opcua config init",
				pair.CertPath, pair.KeyPath)
		} else {
			profile := file.Upsert(settings.Profile)
			profile.Security.Certificate = pair.CertPath
			profile.Security.PrivateKey = pair.KeyPath
			if err := file.Save(); err != nil {
				return err
			}
			streams.Out.Notef("profile %q now uses this certificate", settings.Profile)
		}
	}

	streams.Err.Warnf("the server will reject this certificate until an operator trusts it; connect once, approve it, then connect again")
	return nil
}

// certShow inspects a certificate: a local file, or the one the server presents.
func certShow(ctx context.Context, cmd *cobra.Command, commandIO cligen.IO, p cligen.CertShowParams) (cligen.CertificateInfo, error) {
	streams := newOutput(cmd, commandIO)

	if p.File != "" {
		der, err := pki.Load(p.File)
		if err != nil {
			return cligen.CertificateInfo{}, fmt.Errorf("%w: %w", ErrNotFound, err)
		}
		info, err := pki.Describe(der, p.File)
		if err != nil {
			return cligen.CertificateInfo{}, err
		}
		if !machineReadable(string(p.Format)) {
			streams.Out.Title(filepath.Base(p.File), p.File)
			renderCertificate(streams.Out, info)
		}
		return info, nil
	}

	settings, _, err := conn.FromCommand(cmd, commandIO.In)
	if err != nil {
		return cligen.CertificateInfo{}, err
	}

	descriptions, err := opcua.GetEndpoints(ctx, settings.Endpoint(), opcua.DialTimeout(settings.DialTimeout))
	if err != nil {
		return cligen.CertificateInfo{}, fmt.Errorf("%w: get endpoints from %s: %w", ErrConnection, settings.Endpoint(), err)
	}

	// Every secured endpoint of a server presents the same application
	// certificate, so the first one that carries a certificate is the answer.
	for _, description := range descriptions {
		if description == nil || len(description.ServerCertificate) == 0 {
			continue
		}
		info, err := pki.Describe(description.ServerCertificate, "")
		if err != nil {
			return cligen.CertificateInfo{}, err
		}
		if !machineReadable(string(p.Format)) {
			streams.Out.Title(settings.Endpoint(), conn.PolicyName(description.SecurityPolicyURI))
			renderCertificate(streams.Out, info)
		}
		return info, nil
	}

	return cligen.CertificateInfo{}, fmt.Errorf("%w: the server at %s presents no certificate, which means it offers no secured endpoint",
		ErrNotFound, settings.Endpoint())
}

// renderCertificate prints a certificate summary, leading with the fields that
// decide whether a secure channel will work: the validity window, and the
// application URI a server matches against.
func renderCertificate(p *render.Printer, info cligen.CertificateInfo) {
	validity := p.T.Styles.Good
	window := fmt.Sprintf("%s to %s",
		info.NotBefore.Local().Format("2006-01-02"),
		info.NotAfter.Local().Format("2006-01-02"))
	if info.Expired != nil && *info.Expired {
		validity = p.T.Styles.Bad
		window += "  (outside its validity window)"
	} else if time.Until(info.NotAfter) < 30*24*time.Hour {
		validity = p.T.Styles.Uncertain
		window += fmt.Sprintf("  (expires in %s)", time.Until(info.NotAfter).Round(time.Hour))
	}

	fields := []render.Field{
		render.F("Subject", info.Subject),
		render.F("Issuer", info.Issuer),
		render.FS("Valid", window, validity),
	}
	if info.ApplicationURI != nil {
		fields = append(fields, render.F("Application URI", *info.ApplicationURI))
	} else {
		fields = append(fields, render.FS("Application URI", "none - OPC UA requires one in a subject alternative name",
			p.T.Styles.Uncertain))
	}
	if len(info.DNSNames) > 0 {
		fields = append(fields, render.F("DNS names", strings.Join(info.DNSNames, ", ")))
	}
	if len(info.IPAddresses) > 0 {
		fields = append(fields, render.F("IP addresses", strings.Join(info.IPAddresses, ", ")))
	}
	if info.SelfSigned != nil {
		fields = append(fields, render.F("Self-signed", strconv.FormatBool(*info.SelfSigned)))
	}
	if info.KeyBits != nil {
		fields = append(fields, render.F("Key", fmt.Sprintf("%s %d bits",
			derefOr(info.PublicKeyAlgorithm, ""), *info.KeyBits)))
	}
	if info.SignatureAlgorithm != nil {
		fields = append(fields, render.F("Signature", *info.SignatureAlgorithm))
	}
	if info.SerialNumber != nil {
		fields = append(fields, render.F("Serial", *info.SerialNumber))
	}
	if info.ThumbprintSha1 != nil {
		fields = append(fields, render.F("SHA-1", *info.ThumbprintSha1))
	}
	if info.ThumbprintSha256 != nil {
		fields = append(fields, render.F("SHA-256", *info.ThumbprintSha256))
	}
	if len(info.KeyUsage) > 0 {
		fields = append(fields, render.F("Key usage", strings.Join(info.KeyUsage, ", ")))
	}
	if len(info.ExtendedKeyUsage) > 0 {
		fields = append(fields, render.F("Extended usage", strings.Join(info.ExtendedKeyUsage, ", ")))
	}

	p.KV(fields)
}
