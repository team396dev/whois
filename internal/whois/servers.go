package whois

import "time"

type TLDConfig struct {
	Server         string
	RegistrarField []string
	QueryPrefix    string
	RateLimit      time.Duration
}

var TLDConfigs = map[string]TLDConfig{
	"nl":  {Server: "whois.domain-registry.nl:43", RegistrarField: []string{"Registrar:"}},
	"fi":  {Server: "whois.fi:43", RegistrarField: []string{"registrar:"}},
	"dk":  {Server: "whois.dk-hostmaster.dk:43", RegistrarField: []string{"Registrar:"}},
	"cz":  {Server: "whois.nic.cz:43", RegistrarField: []string{"registrar:"}},
	"es":  {Server: "whois.nic.es:43", RegistrarField: []string{"Registrar Name:", "Registrar:"}, RateLimit: 2 * time.Second},
	"pl":  {Server: "whois.dns.pl:43", RegistrarField: []string{"REGISTRAR:", "registrar:"}},
	"at":  {Server: "whois.nic.at:43", RegistrarField: []string{"registrar:"}},
	"it":  {Server: "whois.nic.it:43", RegistrarField: []string{"Registrar", "registrar:"}},
	"com": {Server: "whois.verisign-grs.com:43", RegistrarField: []string{"Registrar:"}},
	"net": {Server: "whois.verisign-grs.com:43", RegistrarField: []string{"Registrar:"}},
	"org": {Server: "whois.pir.org:43", RegistrarField: []string{"Registrar:"}},
	"io":  {Server: "whois.nic.io:43", RegistrarField: []string{"Registrar:"}},
	"de":  {Server: "whois.denic.de:43", QueryPrefix: "-T dn,ace", RegistrarField: []string{"Registrar:"}},
	"uk":  {Server: "whois.nic.uk:43", RegistrarField: []string{"Registrar:"}},
	"ru":  {Server: "whois.tcinet.ru:43", RegistrarField: []string{"registrar:"}},
	"eu":  {Server: "whois.eu:43", RegistrarField: []string{"Registrar:"}},
	"fr":  {Server: "whois.nic.fr:43", RegistrarField: []string{"registrar:"}},
	"be":  {Server: "whois.dns.be:43", RegistrarField: []string{"Registrar:"}},
	"ch":  {Server: "whois.nic.ch:43", RegistrarField: []string{"Registrar:"}},
	"se":  {Server: "whois.iis.se:43", RegistrarField: []string{"registrar:"}},
	"no":  {Server: "whois.norid.no:43", RegistrarField: []string{"Registrar Handle:", "registrar:"}},
	"info": {Server: "whois.afilias.net:43", RegistrarField: []string{"Registrar:"}},
	"biz":  {Server: "whois.neulevel.biz:43", RegistrarField: []string{"Registrar:"}},
}
