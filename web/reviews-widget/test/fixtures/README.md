# Test fixtures

`shegida-product.html` is a saved copy of a live Yandex Kit product page:
`https://shegida.ru/products/svitshot-hlopkovyiy-100954`, seller article `107`.

It is used to test `loader.js` injection offline. The page contains `"sku":"107"`
in its structured data and hydration state. Class names are hashed CSS modules and
must not be used as injection anchors. Re-download if the storefront layout changes.
