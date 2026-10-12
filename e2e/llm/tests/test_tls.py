import os
import sys
import unittest
import unittest.mock

sys.path.insert(0, os.path.dirname(os.path.dirname(os.path.abspath(__file__))))

import providers  # noqa: E402

SYSTEM_BUNDLE = next((p for p in providers.SYSTEM_CA_BUNDLES if os.path.exists(p)), None)


@unittest.skipUnless(SYSTEM_BUNDLE, "no system CA bundle on this machine")
class TlsContextTest(unittest.TestCase):
    def test_an_empty_default_store_borrows_the_systems_bundle(self):
        # python.org's macOS build ships no CA certificates until its installer
        # script is run, and every HTTPS call then fails verification.
        context = providers.tls_context(default_store_empty=True)
        self.assertGreater(context.cert_store_stats()["x509_ca"], 0)

    def test_an_empty_store_is_detected_without_being_told(self):
        import ssl
        empty = lambda: ssl.SSLContext(ssl.PROTOCOL_TLS_CLIENT)  # noqa: E731 — a store with no CA
        with unittest.mock.patch.object(providers.ssl, "create_default_context", empty):
            context = providers.tls_context()
        self.assertGreater(context.cert_store_stats()["x509_ca"], 0)

    def test_no_protocol_older_than_tls_1_2_is_offered(self):
        import ssl
        self.assertGreaterEqual(providers.tls_context().minimum_version, ssl.TLSVersion.TLSv1_2)

    def test_verification_is_never_switched_off(self):
        import ssl
        context = providers.tls_context(default_store_empty=True)
        self.assertEqual(context.verify_mode, ssl.CERT_REQUIRED)
        self.assertTrue(context.check_hostname)


if __name__ == "__main__":
    unittest.main()
