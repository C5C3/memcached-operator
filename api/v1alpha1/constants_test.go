package v1alpha1

// Test constants shared by the test files of this package. Each name states
// the value's meaning, so one value can have several constants.
const (
	testAPIGroup               = "memcached.c5c3.io"
	testAPIVersion             = "memcached.c5c3.io/v1alpha1"
	testAnnotationValueScrape  = "true"
	testCRDSingularName        = "memcached"
	testConditionReasonReady   = "Ready"
	testConditionTypeAvailable = "Available"
	testDeprecationWarning     = "memcached.c5c3.io/v1alpha1 Memcached is deprecated; use memcached.c5c3.io/v1beta1 Memcached"
	testExtraArgModern         = "modern"
	testKindMemcached          = "Memcached"
	testKindMemcachedList      = "MemcachedList"
	testLabelKeyApp            = "app"
	testLabelKeyEnv            = "env"
	testLabelValueApp          = "memcached"
	testLabelValueEnv          = "prod"
	testListItemName           = "mc-0"
	testMinimalObjectName      = "minimal"
	testNamespaceDefault       = "default"
	testNamespaceProd          = "prod"
	testObjectName             = "test-mc"
	testPrinterColumnReady     = "Ready"
	testPropertyMemcached      = "memcached"
	testPropertySpec           = "spec"
	testServerAddress          = "10.244.0.5:11211"
	testTLSCertSecretName      = "tls-cert"
	testTLSSecretName          = "tls-secret"
	testTopologyKeyZone        = "topology.kubernetes.io/zone"
	testVersion                = "v1alpha1"
)
