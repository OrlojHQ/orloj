# SealedSecret

> **Stability: beta** -- This resource kind ships with `orloj.dev/v1` and is suitable for production use, but its schema may evolve with migration guidance in future minor releases.

`SealedSecret` is the git-safe counterpart to `Secret`. It stores encrypted secret entries that only `orlojd` can decrypt, then reconciles them into a normal `Secret` with the same name and namespace.

## spec

- `encryptedData` (map[string]object): encrypted secret entries keyed by final secret key name.
  - `keyId` (string, required): active sealing key identifier used to encrypt this entry.
  - `wrappedKey` (string, required): base64 RSA-OAEP wrapped AES data key.
  - `ciphertext` (string, required): base64 `nonce || aes_gcm_ciphertext`.
- `template.labels` (map[string]string): labels copied onto the generated `Secret`.
- `template.annotations` (map[string]string): annotations copied onto the generated `Secret`.

In v1, the generated `Secret` always uses the same `metadata.name` and `metadata.namespace` as the `SealedSecret`.

## status

- `phase` (string): `Pending`, `Ready`, or `Error`.
- `lastError` (string): controller-visible decrypt, key, or ownership conflict error.
- `observedGeneration` (int64): latest generation processed by the controller.

## Controller behavior

- `orlojd` decrypts `spec.encryptedData` using the active sealing private key.
- The resulting `Secret` is written through the normal `Secret` store path, so existing consumers and worker secret resolution do not change.
- Generated Secrets are annotated with `orloj.dev/sealedsecret-owner=<namespace>/<name>`.
- If a target `Secret` already exists without that ownership annotation, reconcile fails closed and `status.phase` becomes `Error`.
- A background orphan cleanup pass removes generated Secrets whose source `SealedSecret` no longer exists.

## API Endpoints

- `POST /v1/sealed-secrets`
- `GET /v1/sealed-secrets`
- `GET /v1/sealed-secrets/{name}`
- `PUT /v1/sealed-secrets/{name}`
- `DELETE /v1/sealed-secrets/{name}`
- `GET /v1/sealing-key/public`

Public key response:

```json
{
  "keyId": "4d8e4f1f7c2b8b27d6f2e2f8d1fef3c5",
  "algorithm": "rsa-oaep-sha256+aes-256-gcm",
  "publicKeyPEM": "-----BEGIN PUBLIC KEY-----\n...\n-----END PUBLIC KEY-----\n"
}
```

## CLI Workflow

Fetch the active public key:

```bash
orlojctl seal public-key
```

Seal a normal `Secret` manifest into a `SealedSecret` manifest:

```bash
orlojctl seal secret -f secret.yaml
```

Seal directly from literals without creating `secret.yaml` first:

```bash
orlojctl seal secret openai-api-key \
  --from-literal value=sk-prod-123 \
  --out secrets/openai-api-key.sealed.yaml
```

Then apply the sealed manifest as usual:

```bash
orlojctl apply -f secret.sealed.yaml
```

For key generation, storage, crypto details, and a comparison with Bitnami's renewal model, see [Sealing Key Security Model](../../operations/security.md#sealing-key-security-model).

See also:

- [Secret](./secret.md)
- [Secret Handling and production guidance](../../operations/security.md#secret-handling)
- [CLI Reference](../cli.md)
- [API Reference](../api.md)
