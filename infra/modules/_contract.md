# Open VCC IaC contract

Both `infra/modules/aws` and `infra/modules/azure` satisfy this exact contract. A consumer that knows one module knows the other.

## Inputs

| Variable | Type | Required | Default | Notes |
|---|---|---|---|---|
| `name` | `string` | yes | — | Resource-name prefix; tags too. |
| `region` | `string` | yes | — | Provider-native region id (e.g. `us-east-1`, `eastus`). |
| `instance_size` | `string` | yes | — | Provider-native size (`t3.small` ↔ `Standard_B2s`). |
| `ssh_public_key` | `string` | yes | — | Inserted into the VM via cloud-init. |
| `ingress_cidrs` | `list(string)` | no | `["0.0.0.0/0"]` | CIDRs allowed to reach `app_port` and 22. |
| `app_port` | `number` | no | `8000` | TCP port the example app listens on. |
| `cloud_init` | `string` | no | `""` | Raw cloud-init / user_data. Empty means none. |
| `tags` | `map(string)` | no | `{}` | Applied as cloud-native tags on every resource that supports them. |

## Outputs

| Output | Type | Notes |
|---|---|---|
| `vm_public_ip` | `string` | The address the engine should target. |
| `vm_private_ip` | `string` | Useful for VPC-peering scenarios (roadmap). |
| `ssh_user` | `string` | `ec2-user` (AWS), `azureuser` (Azure). |
| `cloud` | `string` | Literal `"aws"` or `"azure"`. |
| `region` | `string` | Echoes the input. |
| `vm_id` | `string` | Provider-native VM id. |

## Why "input contract" and not provider abstraction?

We deliberately *don't* hide the cloud-native types behind a generic shim — `instance_size` is a string the user knows belongs to a specific provider catalog. The contract is in the variable names and shapes, not in the values. This keeps the modules thin and avoids leaky abstractions.
