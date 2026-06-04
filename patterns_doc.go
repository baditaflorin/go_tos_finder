package main

// DocType is a canonical legal-document category identifier.
type DocType string

const (
	DocTermsOfService      DocType = "terms_of_service"
	DocPrivacyPolicy       DocType = "privacy_policy"
	DocCookiePolicy        DocType = "cookie_policy"
	DocAcceptableUse       DocType = "acceptable_use"
	DocDMCA                DocType = "dmca"
	DocGDPRDPA             DocType = "gdpr_dpa"
	DocSubprocessors       DocType = "subprocessors"
	DocSLA                 DocType = "sla"
	DocRefundPolicy        DocType = "refund_policy"
	DocShippingPolicy      DocType = "shipping_policy"
	DocImprint             DocType = "imprint"
	DocCommunityGuidelines DocType = "community_guidelines"
	DocCopyrightPolicy     DocType = "copyright_policy"
	DocTrustCenter         DocType = "trust_center"
	DocLegalHub            DocType = "legal_hub"
	DocDisclaimer          DocType = "disclaimer"
)
