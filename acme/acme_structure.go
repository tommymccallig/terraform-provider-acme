package acme

import (
	"crypto"
	"crypto/rand"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/hashicorp/terraform/helper/schema"
	"github.com/xenolf/lego/certcrypto"
	"github.com/xenolf/lego/certificate"
	"github.com/xenolf/lego/challenge"
	"github.com/xenolf/lego/challenge/dns01"
	"github.com/xenolf/lego/lego"
	"github.com/xenolf/lego/providers/dns/acmedns"
	"github.com/xenolf/lego/providers/dns/alidns"
	"github.com/xenolf/lego/providers/dns/auroradns"
	"github.com/xenolf/lego/providers/dns/azure"
	"github.com/xenolf/lego/providers/dns/bluecat"
	"github.com/xenolf/lego/providers/dns/cloudflare"
	"github.com/xenolf/lego/providers/dns/cloudxns"
	"github.com/xenolf/lego/providers/dns/conoha"
	"github.com/xenolf/lego/providers/dns/designate"
	"github.com/xenolf/lego/providers/dns/digitalocean"
	"github.com/xenolf/lego/providers/dns/dnsimple"
	"github.com/xenolf/lego/providers/dns/dnsmadeeasy"
	"github.com/xenolf/lego/providers/dns/dnspod"
	"github.com/xenolf/lego/providers/dns/dreamhost"
	"github.com/xenolf/lego/providers/dns/duckdns"
	"github.com/xenolf/lego/providers/dns/dyn"
	"github.com/xenolf/lego/providers/dns/exec"
	"github.com/xenolf/lego/providers/dns/exoscale"
	"github.com/xenolf/lego/providers/dns/fastdns"
	"github.com/xenolf/lego/providers/dns/gandi"
	"github.com/xenolf/lego/providers/dns/gandiv5"
	"github.com/xenolf/lego/providers/dns/gcloud"
	"github.com/xenolf/lego/providers/dns/glesys"
	"github.com/xenolf/lego/providers/dns/godaddy"
	"github.com/xenolf/lego/providers/dns/hostingde"
	"github.com/xenolf/lego/providers/dns/httpreq"
	"github.com/xenolf/lego/providers/dns/iij"
	"github.com/xenolf/lego/providers/dns/inwx"
	"github.com/xenolf/lego/providers/dns/lightsail"
	"github.com/xenolf/lego/providers/dns/linode"
	"github.com/xenolf/lego/providers/dns/linodev4"
	"github.com/xenolf/lego/providers/dns/mydnsjp"
	"github.com/xenolf/lego/providers/dns/namecheap"
	"github.com/xenolf/lego/providers/dns/namedotcom"
	"github.com/xenolf/lego/providers/dns/netcup"
	"github.com/xenolf/lego/providers/dns/nifcloud"
	"github.com/xenolf/lego/providers/dns/ns1"
	"github.com/xenolf/lego/providers/dns/otc"
	"github.com/xenolf/lego/providers/dns/ovh"
	"github.com/xenolf/lego/providers/dns/pdns"
	"github.com/xenolf/lego/providers/dns/rackspace"
	"github.com/xenolf/lego/providers/dns/rfc2136"
	"github.com/xenolf/lego/providers/dns/route53"
	"github.com/xenolf/lego/providers/dns/sakuracloud"
	"github.com/xenolf/lego/providers/dns/selectel"
	"github.com/xenolf/lego/providers/dns/stackpath"
	"github.com/xenolf/lego/providers/dns/transip"
	"github.com/xenolf/lego/providers/dns/vegadns"
	"github.com/xenolf/lego/providers/dns/vscale"
	"github.com/xenolf/lego/providers/dns/vultr"
	"github.com/xenolf/lego/providers/dns/zoneee"
	"github.com/xenolf/lego/registration"
	"software.sslmate.com/src/go-pkcs12"
)

// baseACMESchema returns a map[string]*schema.Schema with all the elements
// necessary to build the base elements of an ACME resource schema. Use this,
// along with a schema helper of a specific check type, to return the full
// schema.
func baseACMESchema() map[string]*schema.Schema {
	return map[string]*schema.Schema{
		"account_key_pem": {
			Type:      schema.TypeString,
			Required:  true,
			ForceNew:  true,
			Sensitive: true,
		},
	}
}

// registrationSchema returns a map[string]*schema.Schema with all the elements
// that are specific to an ACME registration resource.
func registrationSchema() map[string]*schema.Schema {
	return map[string]*schema.Schema{
		// Even though the ACME spec does allow for multiple contact types, lego
		// only works with a single email address.
		"email_address": {
			Type:     schema.TypeString,
			Required: true,
			ForceNew: true,
		},
		"registration_url": {
			Type:     schema.TypeString,
			Computed: true,
		},
	}
}

// certificateSchema returns a map[string]*schema.Schema with all the elements
// that are specific to an ACME certificate resource.
func certificateSchema() map[string]*schema.Schema {
	return map[string]*schema.Schema{
		"common_name": {
			Type:          schema.TypeString,
			Optional:      true,
			ForceNew:      true,
			ConflictsWith: []string{"certificate_request_pem"},
		},
		"subject_alternative_names": {
			Type:          schema.TypeSet,
			Optional:      true,
			Elem:          &schema.Schema{Type: schema.TypeString},
			Set:           schema.HashString,
			ForceNew:      true,
			ConflictsWith: []string{"certificate_request_pem"},
		},
		"key_type": {
			Type:          schema.TypeString,
			Optional:      true,
			ForceNew:      true,
			Default:       "2048",
			ConflictsWith: []string{"certificate_request_pem"},
			ValidateFunc:  validateKeyType,
		},
		"certificate_request_pem": {
			Type:          schema.TypeString,
			Optional:      true,
			ForceNew:      true,
			ConflictsWith: []string{"common_name", "subject_alternative_names", "key_type"},
		},
		"min_days_remaining": {
			Type:     schema.TypeInt,
			Optional: true,
			Default:  7,
		},
		"dns_challenge": {
			Type:     schema.TypeList,
			Required: true,
			MaxItems: 1,
			Elem: &schema.Resource{
				Schema: map[string]*schema.Schema{
					"provider": {
						Type:     schema.TypeString,
						Required: true,
					},
					"config": {
						Type:         schema.TypeMap,
						Optional:     true,
						ValidateFunc: validateDNSChallengeConfig,
						Sensitive:    true,
					},
					"recursive_nameservers": {
						Type:     schema.TypeList,
						Optional: true,
						Elem:     &schema.Schema{Type: schema.TypeString},
					},
				},
			},
		},
		"must_staple": {
			Type:     schema.TypeBool,
			Optional: true,
			Default:  false,
			ForceNew: true,
		},
		"certificate_url": {
			Type:     schema.TypeString,
			Computed: true,
		},
		"certificate_domain": {
			Type:     schema.TypeString,
			Computed: true,
		},
		"private_key_pem": {
			Type:      schema.TypeString,
			Computed:  true,
			Sensitive: true,
		},
		"certificate_pem": {
			Type:     schema.TypeString,
			Computed: true,
		},
		"issuer_pem": {
			Type:     schema.TypeString,
			Computed: true,
		},
		"certificate_p12": {
			Type:      schema.TypeString,
			Computed:  true,
			Sensitive: true,
		},
	}
}

// registrationSchemaFull returns a merged baseACMESchema + registrationSchema.
func registrationSchemaFull() map[string]*schema.Schema {
	m := baseACMESchema()
	for k, v := range registrationSchema() {
		m[k] = v
	}
	return m
}

// certificateSchemaFull returns a merged baseACMESchema + certificateSchema.
func certificateSchemaFull() map[string]*schema.Schema {
	m := baseACMESchema()
	for k, v := range certificateSchema() {
		m[k] = v
	}
	return m
}

// acmeUser implements acme.User.
type acmeUser struct {

	// The email address for the account.
	Email string

	// The registration resource object.
	Registration *registration.Resource

	// The private key for the account.
	key crypto.PrivateKey
}

func (u acmeUser) GetEmail() string {
	return u.Email
}
func (u acmeUser) GetRegistration() *registration.Resource {
	return u.Registration
}
func (u acmeUser) GetPrivateKey() crypto.PrivateKey {
	return u.key
}

// expandACMEUser creates a new instance of an ACME user from set
// email_address and private_key_pem fields, and a registration
// if one exists.
func expandACMEUser(d *schema.ResourceData) (*acmeUser, error) {
	key, err := privateKeyFromPEM([]byte(d.Get("account_key_pem").(string)))
	if err != nil {
		return nil, err
	}

	user := &acmeUser{
		key: key,
	}

	// only set these email if it's in the schema.
	if v, ok := d.GetOk("email_address"); ok {
		user.Email = v.(string)
	}

	return user, nil
}

// saveACMERegistration takes an *registration.Resource and sets the appropriate fields
// for a registration resource.
func saveACMERegistration(d *schema.ResourceData, reg *registration.Resource) error {
	d.Set("registration_url", reg.URI)

	return nil
}

// expandACMEClient creates a connection to an ACME server from resource data,
// and also returns the user.
//
// If loadReg is supplied, the registration information is loaded in to the
// user's registration, if it exists - if the account cannot be resolved by the
// private key, then the appropriate error is returned.
func expandACMEClient(d *schema.ResourceData, meta interface{}, loadReg bool) (*lego.Client, *acmeUser, error) {
	user, err := expandACMEUser(d)
	if err != nil {
		return nil, nil, fmt.Errorf("Error getting user data: %s", err.Error())
	}

	config := lego.NewConfig(user)
	config.CADirURL = meta.(*Config).ServerURL

	// Note this function is used by both the registration and certificate
	// resources, but key type is not necessary during registration, so
	// it's okay if it's empty for that.
	if v, ok := d.GetOk("key_type"); ok {
		config.Certificate.KeyType = certcrypto.KeyType(v.(string))
	}

	client, err := lego.NewClient(config)
	if err != nil {
		return nil, nil, err
	}

	// Populate user's registration resource if needed
	if loadReg {
		user.Registration, err = client.Registration.ResolveAccountByKey()
		if err != nil {
			return nil, nil, err
		}
	}

	return client, user, nil
}

// certificateResourceExpander is a simple interface to allow us to use the Get
// function that is in ResourceData and ResourceDiff under the same function.
type certificateResourceExpander interface {
	Get(string) interface{}
	GetChange(string) (interface{}, interface{})
}

// expandCertificateResource takes saved state in the certificate resource
// and returns an certificate.Resource.
func expandCertificateResource(d certificateResourceExpander) *certificate.Resource {
	cert := &certificate.Resource{
		Domain:     d.Get("certificate_domain").(string),
		CertURL:    d.Get("certificate_url").(string),
		PrivateKey: []byte(d.Get("private_key_pem").(string)),
		CSR:        []byte(d.Get("certificate_request_pem").(string)),
	}
	// There are situations now where the new certificate may be blank, which
	// signifies that the certificate needs to be renewed. In this case, we need
	// the old value here, versus the new one.
	oldCertPEM, newCertPEM := d.GetChange("certificate_pem")
	if newCertPEM.(string) != "" {
		cert.Certificate = []byte(newCertPEM.(string))
	} else {
		cert.Certificate = []byte(oldCertPEM.(string))
	}
	return cert
}

// saveCertificateResource takes an certificate.Resource and sets fields.
func saveCertificateResource(d *schema.ResourceData, cert *certificate.Resource) error {
	d.Set("certificate_url", cert.CertURL)
	d.Set("certificate_domain", cert.Domain)
	d.Set("private_key_pem", string(cert.PrivateKey))
	issued, issuer, err := splitPEMBundle(cert.Certificate)
	if err != nil {
		return err
	}

	d.Set("certificate_pem", string(issued))
	d.Set("issuer_pem", string(issuer))

	pfxB64, err := bundleToPKCS12(cert.Certificate, cert.PrivateKey)
	if err != nil {
		return err
	}

	d.Set("certificate_p12", string(pfxB64))

	return nil
}

// certDaysRemaining takes an certificate.Resource, parses the
// certificate, and computes the days that it has remaining.
func certDaysRemaining(cert *certificate.Resource) (int64, error) {
	x509Certs, err := parsePEMBundle(cert.Certificate)
	if err != nil {
		return 0, err
	}
	c := x509Certs[0]

	if c.IsCA {
		return 0, fmt.Errorf("First certificate is a CA certificate")
	}

	expiry := c.NotAfter.Unix()
	now := time.Now().Unix()

	return (expiry - now) / 86400, nil
}

// splitPEMBundle gets a slice of x509 certificates from parsePEMBundle,
// and always returns 2 certificates - the issued cert first, and the issuer
// certificate second.
//
// if the certificate count in a bundle is != 2 then this function will fail.
func splitPEMBundle(bundle []byte) (cert, issuer []byte, err error) {
	cb, err := parsePEMBundle(bundle)
	if err != nil {
		return
	}
	if len(cb) != 2 {
		err = fmt.Errorf("Certificate bundle does not contain exactly 2 certificates")
		return
	}
	// lego always returns the issued cert first, if the CA is first there is a problem
	if cb[0].IsCA {
		err = fmt.Errorf("First certificate is a CA certificate")
		return
	}

	cert = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cb[0].Raw})
	issuer = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cb[1].Raw})
	return
}

// bundleToPKCS12 packs an issued certificate (and any supplied
// intermediates) into a PFX file.  The private key is included in
// the archive if it is a non-zero value.
//
// The returned archive is base64-encoded.
func bundleToPKCS12(bundle, key []byte) ([]byte, error) {
	cb, err := parsePEMBundle(bundle)
	if err != nil {
		return nil, err
	}

	if len(cb) != 2 {
		return nil, fmt.Errorf("Certificate bundle does not contain exactly 2 certificates")
	}

	// lego always returns the issued cert first, if the CA is first there is a problem
	if cb[0].IsCA {
		return nil, fmt.Errorf("First certificate is a CA certificate")
	}

	var pk crypto.PrivateKey
	if len(key) > 0 {
		if pk, err = privateKeyFromPEM(key); err != nil {
			return nil, err
		}
	}

	pfxData, err := pkcs12.Encode(rand.Reader, pk, cb[0], cb[1:2], "")
	if err != nil {
		return nil, err
	}

	buf := make([]byte, base64.RawStdEncoding.EncodedLen(len(pfxData)))
	base64.RawStdEncoding.Encode(buf, pfxData)
	return buf, nil
}

// parsePEMBundle parses a certificate bundle from top to bottom and returns
// a slice of x509 certificates. This function will error if no certificates are found.
//
// TODO: This was taken from lego directly, consider exporting it there, or
// consolidating with other TF crypto functions.
func parsePEMBundle(bundle []byte) ([]*x509.Certificate, error) {
	var certificates []*x509.Certificate
	var certDERBlock *pem.Block

	for {
		certDERBlock, bundle = pem.Decode(bundle)
		if certDERBlock == nil {
			break
		}

		if certDERBlock.Type == "CERTIFICATE" {
			cert, err := x509.ParseCertificate(certDERBlock.Bytes)
			if err != nil {
				return nil, err
			}
			certificates = append(certificates, cert)
		}
	}

	if len(certificates) == 0 {
		return nil, errors.New("no certificates were found while parsing the bundle")
	}

	return certificates, nil
}

// helper function to map environment variables if set
func mapEnvironmentVariableValues(keyMapping map[string]string) {
	for key := range keyMapping {
		if value, ok := os.LookupEnv(key); ok {
			os.Setenv(keyMapping[key], value)
		}
	}
}

// setDNSChallenge takes a *lego.Client and the DNS challenge complex
// structure as a map[string]interface{}, and configues the client to
// only allow a DNS challenge with the configured provider.
func setDNSChallenge(client *lego.Client, m map[string]interface{}) error {
	var provider challenge.Provider
	var err error
	var providerName string

	if v, ok := m["provider"]; ok && v.(string) != "" {
		providerName = v.(string)
	} else {
		return fmt.Errorf("DNS challenge provider not defined")
	}
	// Config only needs to be set if it's defined, otherwise existing env/SDK
	// defaults are fine.
	if v, ok := m["config"]; ok {
		for k, v := range v.(map[string]interface{}) {
			os.Setenv(k, v.(string))
		}
	}

	// The below list is manually kept in sync with
	// lego/providers/dns/dns_providers.go
	switch providerName {
	case "acme-dns":
		provider, err = acmedns.NewDNSProvider()
	case "alidns":
		provider, err = alidns.NewDNSProvider()
	case "auroradns":
		provider, err = auroradns.NewDNSProvider()
	case "azure":
		// map terraform provider environment variables if present
		mapEnvironmentVariableValues(map[string]string{
			"ARM_CLIENT_ID":       "AZURE_CLIENT_ID",
			"ARM_CLIENT_SECRET":   "AZURE_CLIENT_SECRET",
			"ARM_SUBSCRIPTION_ID": "AZURE_SUBSCRIPTION_ID",
			"ARM_TENANT_ID":       "AZURE_TENANT_ID",
			"ARM_RESOURCE_GROUP":  "AZURE_RESOURCE_GROUP",
		})
		provider, err = azure.NewDNSProvider()
	case "bluecat":
		provider, err = bluecat.NewDNSProvider()
	case "cloudflare":
		provider, err = cloudflare.NewDNSProvider()
	case "cloudxns":
		provider, err = cloudxns.NewDNSProvider()
	case "conoha":
		provider, err = conoha.NewDNSProvider()
	case "designate":
		provider, err = designate.NewDNSProvider()
	case "digitalocean":
		provider, err = digitalocean.NewDNSProvider()
	case "dnsimple":
		provider, err = dnsimple.NewDNSProvider()
	case "dnsmadeeasy":
		provider, err = dnsmadeeasy.NewDNSProvider()
	case "dnspod":
		provider, err = dnspod.NewDNSProvider()
	case "dreamhost":
		provider, err = dreamhost.NewDNSProvider()
	case "duckdns":
		provider, err = duckdns.NewDNSProvider()
	case "dyn":
		provider, err = dyn.NewDNSProvider()
	case "exec":
		provider, err = exec.NewDNSProvider()
	case "exoscale":
		provider, err = exoscale.NewDNSProvider()
	case "fastdns":
		provider, err = fastdns.NewDNSProvider()
	case "gandi":
		provider, err = gandi.NewDNSProvider()
	case "gandiv5":
		provider, err = gandiv5.NewDNSProvider()
	case "glesys":
		provider, err = glesys.NewDNSProvider()
	case "gcloud":
		provider, err = gcloud.NewDNSProvider()
	case "godaddy":
		provider, err = godaddy.NewDNSProvider()
	case "hostingde":
		provider, err = hostingde.NewDNSProvider()
	case "httpreq":
		provider, err = httpreq.NewDNSProvider()
	case "iij":
		provider, err = iij.NewDNSProvider()
	case "inwx":
		provider, err = inwx.NewDNSProvider()
	case "lightsail":
		provider, err = lightsail.NewDNSProvider()
	case "linode":
		provider, err = linode.NewDNSProvider()
	case "linodev4":
		provider, err = linodev4.NewDNSProvider()
	case "mydnsjp":
		provider, err = mydnsjp.NewDNSProvider()
	case "namecheap":
		provider, err = namecheap.NewDNSProvider()
	case "namedotcom":
		provider, err = namedotcom.NewDNSProvider()
	case "netcup":
		provider, err = netcup.NewDNSProvider()
	case "nifcloud":
		provider, err = nifcloud.NewDNSProvider()
	case "ns1":
		provider, err = ns1.NewDNSProvider()
	case "otc":
		provider, err = otc.NewDNSProvider()
	case "ovh":
		provider, err = ovh.NewDNSProvider()
	case "pdns":
		provider, err = pdns.NewDNSProvider()
	case "rackspace":
		provider, err = rackspace.NewDNSProvider()
	case "rfc2136":
		provider, err = rfc2136.NewDNSProvider()
	case "route53":
		provider, err = route53.NewDNSProvider()
	case "sakuracloud":
		provider, err = sakuracloud.NewDNSProvider()
	case "selectel":
		provider, err = selectel.NewDNSProvider()
	case "stackpath":
		provider, err = stackpath.NewDNSProvider()
	case "transip":
		provider, err = transip.NewDNSProvider()
	case "vegadns":
		provider, err = vegadns.NewDNSProvider()
	case "vscale":
		provider, err = vscale.NewDNSProvider()
	case "vultr":
		provider, err = vultr.NewDNSProvider()
	case "zoneee":
		provider, err = zoneee.NewDNSProvider()
	default:
		return fmt.Errorf("%s: unsupported DNS challenge provider", providerName)
	}
	if err != nil {
		return err
	}

	var opts []dns01.ChallengeOption
	if nameservers := m["recursive_nameservers"].([]interface{}); len(nameservers) > 0 {
		var s []string
		for _, ns := range nameservers {
			s = append(s, ns.(string))
		}

		opts = append(opts, dns01.AddRecursiveNameservers(s))
	}

	if err := client.Challenge.SetDNS01Provider(provider, opts...); err != nil {
		return err
	}

	return nil
}

// stringSlice converts an interface slice to a string slice.
func stringSlice(src []interface{}) []string {
	var dst []string
	for _, v := range src {
		dst = append(dst, v.(string))
	}
	return dst
}

// privateKeyFromPEM converts a PEM block into a crypto.PrivateKey.
func privateKeyFromPEM(pemData []byte) (crypto.PrivateKey, error) {
	var result *pem.Block
	rest := pemData
	for {
		result, rest = pem.Decode(rest)
		if result == nil {
			return nil, fmt.Errorf("Cannot decode supplied PEM data")
		}
		switch result.Type {
		case "RSA PRIVATE KEY":
			return x509.ParsePKCS1PrivateKey(result.Bytes)
		case "EC PRIVATE KEY":
			return x509.ParseECPrivateKey(result.Bytes)
		}
	}
}

// csrFromPEM converts a PEM block into an *x509.CertificateRequest.
func csrFromPEM(pemData []byte) (*x509.CertificateRequest, error) {
	var result *pem.Block
	rest := pemData
	for {
		result, rest = pem.Decode(rest)
		if result == nil {
			return nil, fmt.Errorf("Cannot decode supplied PEM data")
		}
		if result.Type == "CERTIFICATE REQUEST" {
			return x509.ParseCertificateRequest(result.Bytes)
		}
	}
}

// validateKeyType validates a key_type resource parameter is correct.
func validateKeyType(v interface{}, k string) (ws []string, errors []error) {
	value := v.(string)
	found := false
	for _, w := range []string{"P256", "P384", "2048", "4096", "8192"} {
		if value == w {
			found = true
		}
	}
	if found == false {
		errors = append(errors, fmt.Errorf(
			"Certificate key type must be one of P256, P384, 2048, 4096, or 8192"))
	}
	return
}

// validateDNSChallengeConfig ensures that the values supplied to the
// dns_challenge resource parameter in the acme_certificate resource
// are string values only.
func validateDNSChallengeConfig(v interface{}, k string) (ws []string, errors []error) {
	value := v.(map[string]interface{})
	bad := false
	for _, w := range value {
		switch w.(type) {
		case string:
			continue
		default:
			bad = true
		}
	}
	if bad == true {
		errors = append(errors, fmt.Errorf(
			"DNS challenge config map values must be strings only"))
	}
	return
}
