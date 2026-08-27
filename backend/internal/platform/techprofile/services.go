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

// serviceLabels maps a service key to what a reader sees.
var serviceLabels = map[string]string{
	ServiceWebshop:        "Webshop",
	ServiceCustomerPortal: "Kundenportal",
	ServiceCareers:        "Karriereseite",
	ServiceAPI:            "Öffentliche API",
	ServiceVPN:            "VPN-Zugang",
	ServiceMail:           "Eigene Mail-Infrastruktur",
	ServiceFileCloud:      "Datei-Cloud",
	ServiceDevInfra:       "Entwicklungs-Infrastruktur",
	ServiceStatusPage:     "Statusseite",
	ServiceSupport:        "Support-Portal",
}

// serviceLabelAllowlist maps a subdomain's FIRST label to the service it
// reveals. German and English spellings both appear because the market is
// both.
//
// THIS MAP IS THE PRIVACY BOUNDARY, and it is an allowlist for that reason
// rather than for tidiness. Certificate transparency publishes every hostname
// a company has ever had a certificate for, and those include people:
// `jan.example.de` for a developer's machine, a name in a vanity host, a
// contractor's test box. A denylist would have to anticipate every name a
// person can have. This map passes through only labels that name a SERVICE,
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

// ServiceLabel is what a reader sees for a service key, and false when the key
// names no service this package knows.
//
// Exported so a caller that remembered a key can re-derive the label rather
// than storing it: a label reworded here then takes effect everywhere on the
// next read, instead of surviving in whatever cached it.
func ServiceLabel(key string) (string, bool) {
	label, known := serviceLabels[key]
	return label, known
}

// OperatedServices reads the services a domain's certificate hostnames reveal.
//
// Only the first label is consulted, and only against the allowlist above.
// Everything else — every hostname that does not name a known service — is
// dropped here and never travels further, which is what keeps a person's name
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
		label, _, found := strings.Cut(strings.TrimSuffix(hostname, "."+domain), ".")
		if !found {
			// A single-label subdomain: the whole remainder IS the label.
			label = strings.TrimSuffix(hostname, "."+domain)
		}
		service, allowed := serviceLabelAllowlist[label]
		if !allowed {
			continue
		}
		// The shortest proving hostname wins, so `shop.example.de` is cited
		// rather than `shop.staging.old.example.de` when both exist.
		if existing, seen := best[service]; !seen || len(hostname) < len(existing) {
			best[service] = hostname
		}
	}
	services := make([]Signal, 0, len(best))
	for service, evidence := range best {
		services = append(services, Signal{
			Field: FieldOperatedService, Key: service,
			Label: serviceLabels[service], Evidence: evidence,
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
