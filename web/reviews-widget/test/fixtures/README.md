# Test fixtures

`kit-product-sample.html` is a saved copy of a live Yandex Kit product page
(seller article `107`), with shop-specific strings anonymized.

It is used to test `loader.js` injection offline. The page contains `"sku":"107"`
in its structured data and hydration state. Class names are hashed CSS modules and
must not be used as injection anchors. Re-download if the storefront layout changes.
