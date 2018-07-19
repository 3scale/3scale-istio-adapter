package threescaleClient


// Structs generated from the current configuration in 3scale.


type Config struct {
	ProxyConfig ProxyConfig `json:"proxy_config"`
}

type ProxyConfig struct {
	ID          int64   `json:"id"`
	Version     int64   `json:"version"`
	Environment string  `json:"environment"`
	Content     Content `json:"content"`
}

type Content struct {
	ID                          int64       `json:"id"`
	AccountID                   int64       `json:"account_id"`
	Name                        string      `json:"name"`
	OnelineDescription          interface{} `json:"oneline_description"`
	Description                 interface{} `json:"description"`
	TxtAPI                      interface{} `json:"txt_api"`
	TxtSupport                  interface{} `json:"txt_support"`
	TxtFeatures                 interface{} `json:"txt_features"`
	CreatedAt                   string      `json:"created_at"`
	UpdatedAt                   string      `json:"updated_at"`
	LogoFileName                interface{} `json:"logo_file_name"`
	LogoContentType             interface{} `json:"logo_content_type"`
	LogoFileSize                interface{} `json:"logo_file_size"`
	State                       string      `json:"state"`
	IntentionsRequired          bool        `json:"intentions_required"`
	DraftName                   string      `json:"draft_name"`
	Infobar                     interface{} `json:"infobar"`
	Terms                       interface{} `json:"terms"`
	DisplayProviderKeys         bool        `json:"display_provider_keys"`
	TechSupportEmail            interface{} `json:"tech_support_email"`
	AdminSupportEmail           interface{} `json:"admin_support_email"`
	CreditCardSupportEmail      interface{} `json:"credit_card_support_email"`
	BuyersManageApps            bool        `json:"buyers_manage_apps"`
	BuyersManageKeys            bool        `json:"buyers_manage_keys"`
	CustomKeysEnabled           bool        `json:"custom_keys_enabled"`
	BuyerPlanChangePermission   string      `json:"buyer_plan_change_permission"`
	BuyerCanSelectPlan          bool        `json:"buyer_can_select_plan"`
	NotificationSettings        interface{} `json:"notification_settings"`
	DefaultApplicationPlanID    int64       `json:"default_application_plan_id"`
	DefaultServicePlanID        int64       `json:"default_service_plan_id"`
	DefaultEndUserPlanID        interface{} `json:"default_end_user_plan_id"`
	EndUserRegistrationRequired bool        `json:"end_user_registration_required"`
	TenantID                    int64       `json:"tenant_id"`
	SystemName                  string      `json:"system_name"`
	BackendVersion              string      `json:"backend_version"`
	MandatoryAppKey             bool        `json:"mandatory_app_key"`
	BuyerKeyRegenerateEnabled   bool        `json:"buyer_key_regenerate_enabled"`
	SupportEmail                string      `json:"support_email"`
	ReferrerFiltersRequired     bool        `json:"referrer_filters_required"`
	DeploymentOption            string      `json:"deployment_option"`
	Proxiable                   bool        `json:"proxiable?"`
	BackendAuthenticationType   string      `json:"backend_authentication_type"`
	BackendAuthenticationValue  string      `json:"backend_authentication_value"`
	Proxy                       Proxy       `json:"proxy"`
}

type Proxy struct {
	ID                         int64         `json:"id"`
	TenantID                   int64         `json:"tenant_id"`
	ServiceID                  int64         `json:"service_id"`
	Endpoint                   string        `json:"endpoint"`
	DeployedAt                 interface{}   `json:"deployed_at"`
	APIBackend                 string        `json:"api_backend"`
	AuthAppKey                 string        `json:"auth_app_key"`
	AuthAppID                  string        `json:"auth_app_id"`
	AuthUserKey                string        `json:"auth_user_key"`
	CredentialsLocation        string        `json:"credentials_location"`
	ErrorAuthFailed            string        `json:"error_auth_failed"`
	ErrorAuthMissing           string        `json:"error_auth_missing"`
	CreatedAt                  string        `json:"created_at"`
	UpdatedAt                  string        `json:"updated_at"`
	ErrorStatusAuthFailed      int64         `json:"error_status_auth_failed"`
	ErrorHeadersAuthFailed     string        `json:"error_headers_auth_failed"`
	ErrorStatusAuthMissing     int64         `json:"error_status_auth_missing"`
	ErrorHeadersAuthMissing    string        `json:"error_headers_auth_missing"`
	ErrorNoMatch               string        `json:"error_no_match"`
	ErrorStatusNoMatch         int64         `json:"error_status_no_match"`
	ErrorHeadersNoMatch        string        `json:"error_headers_no_match"`
	SecretToken                string        `json:"secret_token"`
	HostnameRewrite            string        `json:"hostname_rewrite"`
	OauthLoginURL              interface{}   `json:"oauth_login_url"`
	SandboxEndpoint            string        `json:"sandbox_endpoint"`
	APITestPath                string        `json:"api_test_path"`
	APITestSuccess             bool          `json:"api_test_success"`
	ApicastConfigurationDriven bool          `json:"apicast_configuration_driven"`
	OidcIssuerEndpoint         interface{}   `json:"oidc_issuer_endpoint"`
	LockVersion                int64         `json:"lock_version"`
	AuthenticationMethod       string        `json:"authentication_method"`
	HostnameRewriteForSandbox  string        `json:"hostname_rewrite_for_sandbox"`
	EndpointPort               int64         `json:"endpoint_port"`
	Valid                      bool          `json:"valid?"`
	ServiceBackendVersion      string        `json:"service_backend_version"`
	Hosts                      []string      `json:"hosts"`
	Backend                    Backend       `json:"backend"`
	PolicyChain                []PolicyChain `json:"policy_chain"`
	ProxyRules                 []ProxyRule   `json:"proxy_rules"`
}

type Backend struct {
	Endpoint string `json:"endpoint"`
	Host     string `json:"host"`
}

type PolicyChain struct {
	Name          string        `json:"name"`
	Version       string        `json:"version"`
	Configuration Configuration `json:"configuration"`
}

type Configuration struct {
}

type ProxyRule struct {
	ID                    int64         `json:"id"`
	ProxyID               int64         `json:"proxy_id"`
	HTTPMethod            string        `json:"http_method"`
	Pattern               string        `json:"pattern"`
	MetricID              int64         `json:"metric_id"`
	MetricSystemName      string        `json:"metric_system_name"`
	Delta                 int64         `json:"delta"`
	TenantID              int64         `json:"tenant_id"`
	CreatedAt             string        `json:"created_at"`
	UpdatedAt             string        `json:"updated_at"`
	RedirectURL           interface{}   `json:"redirect_url"`
	Parameters            []interface{} `json:"parameters"`
	QuerystringParameters Configuration `json:"querystring_parameters"`
}

