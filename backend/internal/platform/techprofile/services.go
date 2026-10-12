// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package techprofile

import (
	"net"
	"sort"
	"strings"
)

// The services a subdomain can reveal. Each is something a rep can act on:
// a webshop means the company sells online, a careers page means they are
// hiring, a customer portal means they already run self-service.
const (
	ServiceWebshop        = "webshop"
	ServiceCustomerPortal = "customer_portal"
	ServiceCareers        = "careers"
	ServiceAPI            = "api"
	ServiceVPN            = "vpn"
	ServiceMail           = "mail_infrastructure"
	ServiceFileCloud      = "file_cloud"
	ServiceDevInfra       = "dev_infrastructure"
	ServiceStatusPage     = "status_page"
	ServiceSupport        = "support_site"
)

// operatedServices is the closed set of services this package classifies.
//
// A service carries no label of its own. The label is stored on a record, so
// it is written where the record's language is known, and this package
// classifies rather than translates.
var operatedServices = map[string]bool{
	ServiceWebshop:        true,
	ServiceCustomerPortal: true,
	ServiceCareers:        true,
	ServiceAPI:            true,
	ServiceVPN:            true,
	ServiceMail:           true,
	ServiceFileCloud:      true,
	ServiceDevInfra:       true,
	ServiceStatusPage:     true,
	ServiceSupport:        true,
}

// serviceLabelAllowlist maps a subdomain's FIRST label to the service it
// reveals. German and English spellings both appear because the market is
// both.
//
// THIS MAP IS THE PRIVACY BOUNDARY, and it is an allowlist for that reason
// rather than for tidiness. Certificate transparency publishes every hostname
// a company has ever had a certificate for, and those include contacts:
// `jan.example.de` for a developer's machine, a name in a vanity host, a
// contractor's test box. A denylist would have to anticipate every name a
// contact can have. This map passes through only labels that name a SERVICE,
// so a personal name cannot be matched by construction — it is dropped before
// it reaches a cache, a fact row, or a log line.
var serviceLabelAllowlist = map[string]string{
	"shop":         ServiceWebshop,
	"store":        ServiceWebshop,
	"webshop":      ServiceWebshop,
	"onlineshop":   ServiceWebshop,
	"portal":       ServiceCustomerPortal,
	"kunden":       ServiceCustomerPortal,
	"kundenportal": ServiceCustomerPortal,
	"my":           ServiceCustomerPortal,
	"mein":         ServiceCustomerPortal,
	"login":        ServiceCustomerPortal,
	"karriere":     ServiceCareers,
	"careers":      ServiceCareers,
	"jobs":         ServiceCareers,
	"bewerbung":    ServiceCareers,
	"api":          ServiceAPI,
	"vpn":          ServiceVPN,
	"remote":       ServiceVPN,
	"mail":         ServiceMail,
	"smtp":         ServiceMail,
	"imap":         ServiceMail,
	"webmail":      ServiceMail,
	"autodiscover": ServiceMail,
	"cloud":        ServiceFileCloud,
	"nextcloud":    ServiceFileCloud,
	"owncloud":     ServiceFileCloud,
	"git":          ServiceDevInfra,
	"gitlab":       ServiceDevInfra,
	"jenkins":      ServiceDevInfra,
	"grafana":      ServiceDevInfra,
	"jira":         ServiceDevInfra,
	"status":       ServiceStatusPage,
	"support":      ServiceSupport,
	"help":         ServiceSupport,
	"docs":         ServiceSupport,
}

// IsService says whether a key names a service this package classifies.
func IsService(key string) bool {
	return operatedServices[key]
}

// ServiceKeys lists the classifiable service keys, sorted. Exported so a
// surface that renders its own label per service can walk the owner's set
// rather than keep a copy of it.
func ServiceKeys() []string {
	keys := make([]string, 0, len(operatedServices))
	for key := range operatedServices {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// OperatedServices reads the services a domain's certificate hostnames reveal.
//
// Only the first label is consulted, and only against the allowlist above.
// Everything else — every hostname that does not name a known service — is
// dropped here and never travels further, which is what keeps a contact's name
// out of the record.
//
// The result is sorted by service key so two passes over the same certificate
// set produce the same order, which is what lets a caller compare them.
func OperatedServices(domain string, hostnames []string) []Signal {
	domain = strings.ToLower(strings.TrimSpace(domain))
	best := map[string]string{}
	for _, hostname := range hostnames {
		hostname = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(hostname), "."))
		if hostname == "" || hostname == domain {
			continue
		}
		// EXACTLY ONE label under the domain, and that is the privacy rule
		// rather than a tidiness one. Reading only the first label of a deeper
		// name admits `shop.jan-mueller.example.de` as a webshop and then
		// stores the whole hostname — a contact's name — as the evidence that
		// proved it. A service a company operates is published at one label;
		// anything deeper is somebody's box.
		sub := strings.TrimSuffix(hostname, "."+domain)
		if sub == hostname || strings.Contains(sub, ".") {
			continue
		}
		service, allowed := serviceLabelAllowlist[sub]
		if !allowed {
			continue
		}
		// The shortest proving hostname wins among equals.
		if existing, seen := best[service]; !seen || len(hostname) < len(existing) {
			best[service] = hostname
		}
	}
	services := make([]Signal, 0, len(best))
	for service, evidence := range best {
		services = append(services, Signal{
			Field: FieldOperatedService, Key: service, Evidence: evidence,
		})
	}
	sort.Slice(services, func(i, j int) bool { return services[i].Key < services[j].Key })
	return services
}

// Hosting providers we can name from a reverse lookup or a CNAME target.
const (
	HostingHetzner     = "hetzner"
	HostingAWS         = "aws"
	HostingCloudflare  = "cloudflare"
	HostingIONOS       = "ionos"
	HostingStrato      = "strato"
	HostingAzure       = "azure"
	HostingGoogleCloud = "google_cloud"
	HostingOVH         = "ovh"
	HostingOther       = "other"
)

// The provider names a reader sees. Named because each appears on several
// suffixes — one provider signs more than one address space.
const (
	labelHetzner     = "Hetzner"
	labelAzure       = "Microsoft Azure"
	labelGoogleCloud = "Google Cloud"
)

// hostingSuffixes maps a hostname suffix to the provider that operates it.
// The German market names come first because that is where the accounts are.
var hostingSuffixes = []struct {
	suffix string
	key    string
	label  string
}{
	{"hetzner.de", HostingHetzner, labelHetzner},
	{"hetzner.com", HostingHetzner, labelHetzner},
	{"your-server.de", HostingHetzner, labelHetzner},
	{"ionos.de", HostingIONOS, "IONOS"},
	{"1und1.de", HostingIONOS, "IONOS"},
	{"kasserver.com", HostingStrato, "STRATO"},
	{"strato.de", HostingStrato, "STRATO"},
	{"ovh.net", HostingOVH, "OVH"},
	{"amazonaws.com", HostingAWS, "AWS"},
	{"cloudfront.net", HostingAWS, "AWS"},
	{"cloudflare.com", HostingCloudflare, "Cloudflare"},
	{"cloudflare.net", HostingCloudflare, "Cloudflare"},
	{"azure.com", HostingAzure, labelAzure},
	{"windows.net", HostingAzure, labelAzure},
	{"azureedge.net", HostingAzure, labelAzure},
	{"googleusercontent.com", HostingGoogleCloud, labelGoogleCloud},
	{"1e100.net", HostingGoogleCloud, labelGoogleCloud},
	{"ghs.google.com", HostingGoogleCloud, labelGoogleCloud},
}

// HostingProvider reads the hosting provider from the names a domain's
// addresses reverse-resolve to, falling back to the CNAME its host is an alias
// for.
//
// Reverse names are the better evidence: a provider signs its own address
// space, whereas a CNAME only shows what the company pointed at. Both are
// tried because plenty of small sites have no PTR record at all.
//
// An unrecognized name yields no signal rather than `other`. "Hosted by
// somebody we could not name" is not worth a row: it would be true of most
// companies and would tell a rep nothing.
func HostingProvider(reverseNames []string, cnameTarget string) (Signal, bool) {
	candidates := append([]string{}, reverseNames...)
	if cnameTarget != "" {
		candidates = append(candidates, cnameTarget)
	}
	for _, candidate := range candidates {
		candidate = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(candidate), "."))
		if candidate == "" {
			continue
		}
		for _, provider := range hostingSuffixes {
			if candidate == provider.suffix || strings.HasSuffix(candidate, "."+provider.suffix) {
				return Signal{
					Field: FieldHostingProvider, Key: provider.key,
					Label: provider.label, Evidence: candidate,
				}, true
			}
		}
	}
	return Signal{}, false
}

// ReverseLookupTargets picks the addresses worth a reverse lookup.
//
// One per address family is enough: a company's web host answers on one IPv4
// and one IPv6 address, and asking about every address in a round-robin set
// spends the resolver budget to learn the same provider name repeatedly.
func ReverseLookupTargets(addrs []net.IP) []net.IP {
	var targets []net.IP
	var haveV4, haveV6 bool
	for _, addr := range addrs {
		if addr.To4() != nil {
			if haveV4 {
				continue
			}
			haveV4 = true
		} else {
			if haveV6 {
				continue
			}
			haveV6 = true
		}
		targets = append(targets, addr)
	}
	return targets
}
