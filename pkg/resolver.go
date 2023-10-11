package pkg

import (
	"github.com/bepass-org/dnsutils/internal/cache"
	"github.com/bepass-org/dnsutils/internal/config"
	"github.com/bepass-org/dnsutils/internal/resolvers"
	"github.com/bepass-org/dnsutils/internal/statute"
	"strings"
	"time"

	"github.com/miekg/dns"
)

// Options represent a set of common options
// to configure a Resolver.
type Options struct {
	UseIPv4            bool
	UseIPv6            bool
	SearchList         []string
	Ndots              int
	Strategy           string
	Prefer             string
	Timeout            time.Duration
	InsecureSkipVerify bool
	TLSHostname        string
}

// Resolver implements the configuration for a DNS
// Client. Different types of providers can load
// a DNS Resolver satisfying this interface.
type Resolver interface {
	Lookup(dns.Question) (Response, error)
}

// Response represents a custom output format
// for DNS queries. It wraps metadata about the DNS query
// and the DNS Answer as well.
type Response struct {
	Answers     []Answer    `json:"answers"`
	Authorities []Authority `json:"authorities"`
	Questions   []Question  `json:"questions"`
}

type Question struct {
	Name  string `json:"name"`
	Type  string `json:"type"`
	Class string `json:"class"`
}

type Answer struct {
	Name       string `json:"name"`
	Type       string `json:"type"`
	Class      string `json:"class"`
	TTL        string `json:"ttl"`
	Address    string `json:"address"`
	Status     string `json:"status"`
	RTT        string `json:"rtt"`
	Nameserver string `json:"nameserver"`
}

type Authority struct {
	Name       string `json:"name"`
	Type       string `json:"type"`
	Class      string `json:"class"`
	TTL        string `json:"ttl"`
	MName      string `json:"mname"`
	Status     string `json:"status"`
	RTT        string `json:"rtt"`
	Nameserver string `json:"nameserver"`
}

var resolver Resolver

func init() {
	opts := Options{
		UseIPv4:            config.Dns.Settings.UseIPv4,
		UseIPv6:            config.Dns.Settings.UseIPv6,
		Prefer:             config.Dns.Prefer,
		SearchList:         config.Dns.Settings.SearchList,
		Ndots:              config.Dns.Settings.Ndots,
		Timeout:            config.Dns.Settings.Timeout,
		InsecureSkipVerify: config.Dns.Settings.InsecureSkipVerify,
		TLSHostname:        config.Dns.Settings.TLSHostname,
	}
	switch config.Dns.Type {
	case "udp":
		statute.Debug("initiating UDP resolver")
		resolver, _ = resolvers.NewClassicResolver(config.Dns.Address,
			resolvers.ClassicResolverOpts{
				UseTCP: false,
				UseTLS: false,
			}, opts)
	case "tcp":
		statute.Debug("initiating TCP resolver")
		resolver, _ = resolvers.NewClassicResolver(config.Dns.Address,
			resolvers.ClassicResolverOpts{
				UseTCP: true,
				UseTLS: false,
			}, opts)
	case "dot":
		statute.Debug("initiating DOT resolver")
		resolver, _ = resolvers.NewClassicResolver(config.Dns.Address,
			resolvers.ClassicResolverOpts{
				UseTCP: true,
				UseTLS: true,
			}, opts)
	case "doh":
		statute.Debug("initiating DOH resolver")
		resolver, _ = resolvers.NewDOHResolver(config.Dns.Address, opts)
	case "crypt":
		statute.Debug("initiating DNSCrypt resolver")
		resolver, _ = resolvers.NewDNSCryptResolver(config.Dns.Address,
			resolvers.DNSCryptResolverOpts{
				UseTCP: true,
			}, opts)
	case "system":
	default:
		statute.Debug("initiating system resolver")
		resolver, _ = resolvers.NewSystemResolver(opts)
	}
}

// CheckHosts checks if a given domain exists in the local resolver's hosts file
// and returns the corresponding IP address if found, or an empty string if not.
func checkHosts(domain string) string {
	for h := range config.Dns.Hosts {
		if config.Dns.Hosts[h].Domain == domain {
			return config.Dns.Hosts[h].IP
		}
	}
	return ""
}

// Resolve resolves the FQDN to an IP address using the specified resolution mechanism.
func Resolve(fqdn string) (string, error) {
	if h := checkHosts(fqdn); h != "" {
		return h, nil
	}

	// Ensure fqdn ends with a period
	if !strings.HasSuffix(fqdn, ".") {
		fqdn += "."
	}

	// Check the cache for fqdn
	if cachedValue, _ := cache.Get(fqdn); cachedValue != nil {
		statute.Infof("using cached value for %s", fqdn)
		return cachedValue.(string), nil
	}

	question := dns.Question{
		Name:   fqdn,
		Qtype:  dns.TypeA,
		Qclass: dns.ClassINET,
	}

	response, err := resolver.Lookup(question)
	if err != nil {
		return "", err
	}
	statute.Infof("resolved %s to %s", fqdn, response.Answers[0].Address)
	if response.Answers[0].Type == "CNAME" {
		ip, err := Resolve(response.Answers[0].Address)
		if err != nil {
			return "", err
		}
		cache.Set(fqdn, ip)
		return ip, nil
	}
	ip := response.Answers[0].Address
	cache.Set(fqdn, ip)
	return ip, nil
}