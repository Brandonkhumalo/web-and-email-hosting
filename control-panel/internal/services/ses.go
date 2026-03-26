package services

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ses"

	"hosting-platform/control-panel/internal/config"
)

// SESService manages Amazon SES domain verification for customer domains.
// Each customer domain must be verified with SES before it can send email
// through the SES relay (Postfix → SES → recipient).
type SESService struct {
	client *ses.Client
	dns    *DNSService
}

// NewSESService creates a new SES service.
func NewSESService(awsCfg aws.Config, cfg *config.Config, dns *DNSService) *SESService {
	// SES might be in a different region than other services
	sesCfg := awsCfg.Copy()
	sesCfg.Region = cfg.SESRegion

	return &SESService{
		client: ses.NewFromConfig(sesCfg),
		dns:    dns,
	}
}

// VerifyDomain starts domain verification with SES and creates the required DNS records.
// This must be called before a customer domain can send email through SES.
func (s *SESService) VerifyDomain(ctx context.Context, domain, zoneID string) error {
	// 1. Request domain identity verification
	verifyOut, err := s.client.VerifyDomainIdentity(ctx, &ses.VerifyDomainIdentityInput{
		Domain: aws.String(domain),
	})
	if err != nil {
		return fmt.Errorf("verify domain identity: %w", err)
	}

	// 2. Add SES verification TXT record to Route53
	verificationToken := *verifyOut.VerificationToken
	err = s.dns.CreateTXTRecord(ctx, zoneID,
		fmt.Sprintf("_amazonses.%s", domain),
		verificationToken,
	)
	if err != nil {
		return fmt.Errorf("create SES verification record: %w", err)
	}

	// 3. Enable DKIM signing via SES
	dkimOut, err := s.client.VerifyDomainDkim(ctx, &ses.VerifyDomainDkimInput{
		Domain: aws.String(domain),
	})
	if err != nil {
		return fmt.Errorf("verify domain dkim: %w", err)
	}

	// 4. Add SES DKIM CNAME records (SES provides 3 tokens)
	for _, token := range dkimOut.DkimTokens {
		err = s.dns.CreateCNAMERecord(ctx, zoneID,
			fmt.Sprintf("%s._domainkey.%s", token, domain),
			fmt.Sprintf("%s.dkim.amazonses.com", token),
		)
		if err != nil {
			return fmt.Errorf("create SES DKIM record: %w", err)
		}
	}

	return nil
}

// CheckVerificationStatus checks if a domain has been verified by SES.
func (s *SESService) CheckVerificationStatus(ctx context.Context, domain string) (string, error) {
	out, err := s.client.GetIdentityVerificationAttributes(ctx, &ses.GetIdentityVerificationAttributesInput{
		Identities: []string{domain},
	})
	if err != nil {
		return "", err
	}

	attrs, ok := out.VerificationAttributes[domain]
	if !ok {
		return "not_started", nil
	}

	return string(attrs.VerificationStatus), nil
}

// DeleteDomainIdentity removes a domain from SES.
func (s *SESService) DeleteDomainIdentity(ctx context.Context, domain string) error {
	_, err := s.client.DeleteIdentity(ctx, &ses.DeleteIdentityInput{
		Identity: aws.String(domain),
	})
	return err
}
