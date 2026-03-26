package services

import (
	"context"
	"fmt"
	"net"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/route53"
	"github.com/aws/aws-sdk-go-v2/service/route53/types"
)

// DNSService manages Route53 hosted zones and DNS records.
type DNSService struct {
	client *route53.Client
}

// NewDNSService creates a DNS service with the Route53 client.
func NewDNSService(awsCfg aws.Config) *DNSService {
	return &DNSService{
		client: route53.NewFromConfig(awsCfg),
	}
}

// CreateHostedZone creates a new Route53 hosted zone for a customer domain.
// Returns the zone ID and nameserver records the customer must set.
func (s *DNSService) CreateHostedZone(ctx context.Context, domain string) (string, []string, error) {
	out, err := s.client.CreateHostedZone(ctx, &route53.CreateHostedZoneInput{
		Name:            aws.String(domain),
		CallerReference: aws.String(fmt.Sprintf("hosting-platform-%s-%d", domain, ctx.Value("request_id"))),
	})
	if err != nil {
		return "", nil, fmt.Errorf("create hosted zone: %w", err)
	}

	// Extract zone ID (format: /hostedzone/Z1234 → Z1234)
	zoneID := *out.HostedZone.Id
	if len(zoneID) > 12 {
		zoneID = zoneID[12:]
	}

	var nameservers []string
	if out.DelegationSet != nil {
		nameservers = out.DelegationSet.NameServers
	}

	return zoneID, nameservers, nil
}

// DeleteHostedZone removes a Route53 hosted zone and all non-default records.
func (s *DNSService) DeleteHostedZone(ctx context.Context, zoneID string) error {
	listOut, err := s.client.ListResourceRecordSets(ctx, &route53.ListResourceRecordSetsInput{
		HostedZoneId: aws.String(zoneID),
	})
	if err != nil {
		return fmt.Errorf("list records: %w", err)
	}

	var changes []types.Change
	for _, rr := range listOut.ResourceRecordSets {
		if rr.Type == types.RRTypeNs || rr.Type == types.RRTypeSoa {
			continue
		}
		changes = append(changes, types.Change{
			Action:            types.ChangeActionDelete,
			ResourceRecordSet: &rr,
		})
	}

	if len(changes) > 0 {
		_, err = s.client.ChangeResourceRecordSets(ctx, &route53.ChangeResourceRecordSetsInput{
			HostedZoneId: aws.String(zoneID),
			ChangeBatch:  &types.ChangeBatch{Changes: changes},
		})
		if err != nil {
			return fmt.Errorf("delete records: %w", err)
		}
	}

	_, err = s.client.DeleteHostedZone(ctx, &route53.DeleteHostedZoneInput{
		Id: aws.String(zoneID),
	})
	return err
}

// VerifyNameservers checks if a domain's NS records match the Route53 zone.
func (s *DNSService) VerifyNameservers(ctx context.Context, domain, zoneID string) (bool, error) {
	zoneOut, err := s.client.GetHostedZone(ctx, &route53.GetHostedZoneInput{
		Id: aws.String(zoneID),
	})
	if err != nil {
		return false, err
	}

	expected := make(map[string]bool)
	for _, ns := range zoneOut.DelegationSet.NameServers {
		expected[ns+"."] = true
	}

	actual, err := net.LookupNS(domain)
	if err != nil {
		return false, nil
	}

	matches := 0
	for _, ns := range actual {
		if expected[ns.Host] {
			matches++
		}
	}

	return matches >= 2, nil
}

// CreateARecord creates a simple A record pointing to an IP address.
// Used to point customer domains to the EC2 Elastic IP.
func (s *DNSService) CreateARecord(ctx context.Context, zoneID, name, ip string) error {
	return s.upsertRecord(ctx, zoneID, name, types.RRTypeA, []string{ip}, 300)
}

// CreateMXRecord adds an MX record pointing to the mail server.
func (s *DNSService) CreateMXRecord(ctx context.Context, zoneID, domain, mailHost string) error {
	return s.upsertRecord(ctx, zoneID, domain, types.RRTypeMx,
		[]string{fmt.Sprintf("10 %s", mailHost)}, 300)
}

// CreateSRVRecord adds an SRV record for email auto-discover.
func (s *DNSService) CreateSRVRecord(ctx context.Context, zoneID, name, target string, port int) error {
	return s.upsertRecord(ctx, zoneID, name, types.RRTypeSrv,
		[]string{fmt.Sprintf("0 1 %d %s", port, target)}, 300)
}

// CreateAliasRecord creates an A record that aliases to an AWS resource.
func (s *DNSService) CreateAliasRecord(ctx context.Context, zoneID, name, targetDNS, targetZoneID string) error {
	_, err := s.client.ChangeResourceRecordSets(ctx, &route53.ChangeResourceRecordSetsInput{
		HostedZoneId: aws.String(zoneID),
		ChangeBatch: &types.ChangeBatch{
			Changes: []types.Change{{
				Action: types.ChangeActionUpsert,
				ResourceRecordSet: &types.ResourceRecordSet{
					Name: aws.String(name),
					Type: types.RRTypeA,
					AliasTarget: &types.AliasTarget{
						DNSName:              aws.String(targetDNS),
						HostedZoneId:         aws.String(targetZoneID),
						EvaluateTargetHealth: false,
					},
				},
			}},
		},
	})
	return err
}

// CreateTXTRecord adds a TXT record (used for SPF, DMARC, SES verification).
func (s *DNSService) CreateTXTRecord(ctx context.Context, zoneID, name, value string) error {
	return s.upsertRecord(ctx, zoneID, name, types.RRTypeTxt,
		[]string{fmt.Sprintf(`"%s"`, value)}, 300)
}

// CreateCNAMERecord adds a CNAME record.
func (s *DNSService) CreateCNAMERecord(ctx context.Context, zoneID, name, value string) error {
	return s.upsertRecord(ctx, zoneID, name, types.RRTypeCname,
		[]string{value}, 300)
}

// upsertRecord creates or updates a DNS record.
func (s *DNSService) upsertRecord(ctx context.Context, zoneID, name string, rrType types.RRType, values []string, ttl int64) error {
	var records []types.ResourceRecord
	for _, v := range values {
		records = append(records, types.ResourceRecord{Value: aws.String(v)})
	}

	_, err := s.client.ChangeResourceRecordSets(ctx, &route53.ChangeResourceRecordSetsInput{
		HostedZoneId: aws.String(zoneID),
		ChangeBatch: &types.ChangeBatch{
			Changes: []types.Change{{
				Action: types.ChangeActionUpsert,
				ResourceRecordSet: &types.ResourceRecordSet{
					Name:            aws.String(name),
					Type:            rrType,
					TTL:             aws.Int64(ttl),
					ResourceRecords: records,
				},
			}},
		},
	})
	return err
}
