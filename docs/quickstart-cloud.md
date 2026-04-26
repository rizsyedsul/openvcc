# Quickstart: AWS + Azure

Brings up two real VMs (one in each cloud) and runs the engine locally pointing at both. ~10 minutes including provisioning time.

## Prerequisites

- AWS account with credentials configured (`aws configure` or env vars)
- Azure account with `az login` completed
- OpenTofu 1.7+
- The `openvcc` binary built or downloaded

## Stateless example

```sh
cd infra/examples/stateless
cp terraform.tfvars.example terraform.tfvars
# edit terraform.tfvars — set ssh_public_key and ingress_cidrs

tofu init
tofu apply
```

Outputs include `aws_vm_public_ip`, `azure_vm_public_ip`, and `openvcc_yaml_path`. The latter is a fully populated `openvcc.yaml`.

Run the engine locally:

```sh
export OPENVCC_ADMIN_TOKEN="dev-token-change-me"
openvcc engine serve --config=./openvcc.yaml
```

Hit it:

```sh
curl -i http://localhost:8080/
```

The response body contains the cloud identity (from the echo app), and the headers carry the engine's `X-Backend` / `X-Cloud`.

## Stateful example (CockroachDB)

```sh
cd infra/examples/stateful
cp terraform.tfvars.example terraform.tfvars
# edit terraform.tfvars
tofu init
tofu apply
```

The AWS VM bootstraps a single-node CRDB cluster; the Azure VM joins it. Each VM also runs the `notes` example pointing at its local CRDB node.

```sh
export OPENVCC_ADMIN_TOKEN="dev-token-change-me"
openvcc engine serve --config=./openvcc.yaml

# write a note via one cloud, read it from either:
curl -X POST -H 'content-type: application/json' \
     -d '{"text":"hello multi-cloud"}' \
     http://localhost:8080/notes
curl http://localhost:8080/notes
```

## Tear down

```sh
tofu destroy
```

## Caveats (v0.1)

- Public-IP CRDB join. Production wants VPC peering + Azure VPN; that's on the roadmap.
- One VM per cloud; the engine itself is a SPOF unless you run multiple instances behind health-checked DNS.
- No HTTPS termination on the engine; pair with a CDN/WAF.
