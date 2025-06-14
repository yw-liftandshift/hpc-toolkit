# Custom Images in the HPC Toolkit

## Introduction

This module uses [Packer](https://www.packer.io/) to create an image within an
HPC Toolkit deployment. Packer operates by provisioning a short-lived VM in
Google Cloud on which it executes scripts to customize the boot disk for
repeated use. The VM's boot disk is specified from a source image that defaults
to the [HPC VM Image][hpcimage]. This Packer "template" supports customization
by the following approaches following a [recommended use](#recommended-use):

* [startup-script metadata][startup-metadata] from [raw string][sss] or
  [file][ssf]
* [Shell scripts][shell] uploaded from the Packer execution
  environment to the VM
* [Ansible playbooks][ansible] uploaded from the Packer
  execution environment to the VM

They can be specified independently of one another, so that anywhere from 1 to
3 solutions can be used simultaneously. In the case that 0 scripts are supplied,
the source boot disk is effectively copied to your project without
customization. This can be useful in scenarios where increased control over the
image maintenance lifecycle is desired or when policies restrict the use of
images to internal projects.

[sss]: #input_startup_script
[ssf]: #input_startup_script_file
[shell]: #input_shell_scripts
[ansible]: #input_ansible_playbooks
[hpcimage]: https://cloud.google.com/compute/docs/instances/create-hpc-vm
[startup-metadata]: https://cloud.google.com/compute/docs/instances/startup-scripts/linux

## Order of execution

The startup script specified in metadata executes in parallel with the other
supported methods. However, the remaining methods execute in a well-defined
order relative to one another.

1. All shell scripts will execute in the configured order
1. After shell scripts complete, all Ansible playbooks will execute in the
   configured order

> **_NOTE:_** if both [startup\_script][sss] and [startup\_script\_file][ssf]
> are specified, then [startup\_script\_file][ssf] takes precedence.

## Recommended use

Because the [metadata startup script executes in parallel](#order-of-execution)
with the other solutions, conflicts can arise, especially when package managers
(`yum` or `apt`) lock their databases during package installation. Therefore,
it is recommended to choose one of the following approaches:

1. Specify _either_ [startup\_script][sss] _or_ [startup\_script\_file][ssf]
   and do not specify [shell\_scripts][shell] or [ansible\_playbooks][ansible].
   * This can be especially useful in [environments that restrict SSH access](#environments-without-ssh-access)
1. Specify any combination of [shell\_scripts][shell] and
   [ansible\_playbooks][ansible] and do not specify [startup\_script][sss] or
   [startup\_script\_file][ssf].

If any of the startup script approaches fail by returning a code other than 0,
Packer will determine that the build has failed and refuse to save the image.

[metaorder]: https://cloud.google.com/compute/docs/instances/startup-scripts/linux#order_of_execution_of_linux_startup_scripts

## External access with SSH

The [shell scripts][shell] and [Ansible playbooks][ansible] customization
solutions both require SSH access to the VM from the Packer execution
environment. SSH access can be enabled one of 2 ways:

1. The VM is created without a public IP address and SSH tunnels are created
   using [Identity-Aware Proxy (IAP)][iaptunnel].
   * Allow [use\_iap](#input_use_iap) to take on its default value of `true`
1. The VM is created with an IP address on the public internet and firewall
   rules allow SSH access from the Packer execution environment.
   * Set `omit_external_ip = false` (or `omit_external_ip: false` in a
     blueprint)
   * Add firewall rules that open SSH to the VM

The Packer template defaults to using to the 1st IAP-based solution because it
is more secure (no exposure to public internet) and because the [Toolkit VPC
module](../../network/vpc/README.md) automatically sets up all necessary
firewall rules for SSH tunneling and outbound-only access to the internet
through [Cloud NAT][cloudnat].

In either SSH solution, customization scripts should be supplied as files in
the [shell\_scripts][shell] and [ansible\_playbooks][ansible] settings.

[iaptunnel]: https://cloud.google.com/iap/docs/using-tcp-forwarding
[cloudnat]: https://cloud.google.com/nat/docs/overview

## Environments without SSH access

Many network environments disallow SSH access to VMs. In these environments, the
[metadata-based startup scripts][startup-metadata] are appropriate because they
execute entirely independently of the Packer execution environment.

In this scenario, a single scripts should be supplied in the form of a string to
the [startup\_script][sss] input variable. This solution integrates well with
Toolkit runners. Runners operate by using a single startup script whose
behavior is extended by downloading and executing a customizable set of runners
from Cloud Storage at startup.

> **_NOTE:_** Packer will attempt to use SSH if either [shell\_scripts][shell]
> or [ansible\_playbooks][ansible] are set to non-empty values. Leave them at
> their default, empty values to ensure access by SSH is disabled.

## Supplying startup script as a string

The [startup\_script][sss] parameter accepts scripts formatted as strings. In
Packer and Terraform, multi-line strings can be specified using [heredoc
syntax](https://www.terraform.io/language/expressions/strings#heredoc-strings)
in an input [Packer variables file][pkrvars] (`*.pkrvars.hcl`) For example,
the following snippet defines a multi-line bash script followed by an integer
representing the size, in GiB, of the resulting image:

```hcl
startup_script = <<-EOT
  #!/bin/bash
  yum install -y epel-release
  yum install -y jq
  EOT

disk_size = 100
```

In a blueprint, the equivalent syntax is:

```yaml
...
    settings:
      startup_script: |
        #!/bin/bash
        yum install -y epel-release
        yum install -y jq
      disk_size: 100
...
```

[pkrvars]: https://www.packer.io/guides/hcl/variables#from-a-file

## Monitoring startup script execution

When using startup script customization, Packer will print very limited output
to the console. For example:

```text
==> example.googlecompute.toolkit_image: Waiting for any running startup script to finish...
==> example.googlecompute.toolkit_image: Startup script not finished yet. Waiting...
==> example.googlecompute.toolkit_image: Startup script not finished yet. Waiting...
==> example.googlecompute.toolkit_image: Startup script, if any, has finished running.
```

Using the default value for [var.scopes][#input_scopes], the output of startup
script execution will be stored in Cloud Logging. It can be examined using the
[Cloud Logging Console][logging-console] or with a [gcloud logging
read][logging-read-docs] command (substituting `<<PROJECT_ID>>` with your
project ID):

```shell
$ gcloud logging --project <<PROJECT_ID>> read \
    'logName="projects/<<PROJECT_ID>>/logs/GCEMetadataScripts" AND jsonPayload.message=~"^startup-script: "' \
    --format="table[box](timestamp, resource.labels.instance_id, jsonPayload.message)" --freshness 2h
```

Note that this command will print **all** startup script entries within the
project within the "freshness" window **in reverse order**. You may need to
identify the instance ID of the Packer VM and filter further by that value using
`gcloud` or `grep`. To print the entries in the order they would have appeared on
your console, we recommend piping the output of this command to the standard
Linux utility `tac`.

[logging-console]: https://console.cloud.google.com/logs/
[logging-read-docs]: https://cloud.google.com/sdk/gcloud/reference/logging/read

## Example

The [included blueprint](../../../examples/image-builder.yaml) demonstrates a
solution that builds an image using:

* The [HPC VM Image][hpcimage] as a base upon which to customize
* A VPC network with firewall rules that allow IAP-based SSH tunnels
* A Toolkit runner that installs a custom script

Please review the [examples README](../../../examples/README.md#image-builderyaml)
for usage instructions.

## License

Copyright 2022 Google LLC

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

     http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.

<!-- BEGINNING OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
## Requirements

No requirements.

## Providers

No providers.

## Modules

No modules.

## Resources

No resources.

## Inputs

| Name | Description | Type | Default | Required |
|------|-------------|------|---------|:--------:|
| <a name="input_accelerator_count"></a> [accelerator\_count](#input\_accelerator\_count) | Number of accelerator cards to attach to the VM; not necessary for families that always include GPUs (A2). | `number` | `null` | no |
| <a name="input_accelerator_type"></a> [accelerator\_type](#input\_accelerator\_type) | Type of accelerator cards to attach to the VM; not necessary for families that always include GPUs (A2). | `string` | `null` | no |
| <a name="input_ansible_playbooks"></a> [ansible\_playbooks](#input\_ansible\_playbooks) | A list of Ansible playbook configurations that will be uploaded to customize the VM image | <pre>list(object({<br>    playbook_file   = string<br>    galaxy_file     = string<br>    extra_arguments = list(string)<br>  }))</pre> | `[]` | no |
| <a name="input_communicator"></a> [communicator](#input\_communicator) | Communicator to use for provisioners that require access to VM ("ssh" or "winrm") | `string` | `null` | no |
| <a name="input_deployment_name"></a> [deployment\_name](#input\_deployment\_name) | HPC Toolkit deployment name | `string` | n/a | yes |
| <a name="input_disk_size"></a> [disk\_size](#input\_disk\_size) | Size of disk image in GB | `number` | `null` | no |
| <a name="input_image_family"></a> [image\_family](#input\_image\_family) | The family name of the image to be built. Defaults to `deployment_name` | `string` | `null` | no |
| <a name="input_image_name"></a> [image\_name](#input\_image\_name) | The name of the image to be built. If not supplied, it will be set to image\_family-$ISO\_TIMESTAMP | `string` | `null` | no |
| <a name="input_labels"></a> [labels](#input\_labels) | Labels to apply to the short-lived VM | `map(string)` | `null` | no |
| <a name="input_machine_type"></a> [machine\_type](#input\_machine\_type) | VM machine type on which to build new image | `string` | `"n2-standard-4"` | no |
| <a name="input_manifest_file"></a> [manifest\_file](#input\_manifest\_file) | File to which to write Packer build manifest | `string` | `"packer-manifest.json"` | no |
| <a name="input_metadata"></a> [metadata](#input\_metadata) | Instance metadata to attach to the build VM (startup-script key overridden by var.startup\_script and var.startup\_script\_file if either is set) | `map(string)` | `{}` | no |
| <a name="input_network_project_id"></a> [network\_project\_id](#input\_network\_project\_id) | Project ID of Shared VPC network | `string` | `null` | no |
| <a name="input_omit_external_ip"></a> [omit\_external\_ip](#input\_omit\_external\_ip) | Provision the image building VM without a public IP address | `bool` | `true` | no |
| <a name="input_on_host_maintenance"></a> [on\_host\_maintenance](#input\_on\_host\_maintenance) | Describes maintenance behavior for the instance. If left blank this will default to `MIGRATE` except the use of GPUs requires it to be `TERMINATE` | `string` | `null` | no |
| <a name="input_project_id"></a> [project\_id](#input\_project\_id) | Project in which to create VM and image | `string` | n/a | yes |
| <a name="input_scopes"></a> [scopes](#input\_scopes) | Service account scopes to attach to the instance. See<br>https://cloud.google.com/compute/docs/access/service-accounts. | `list(string)` | <pre>[<br>  "https://www.googleapis.com/auth/userinfo.email",<br>  "https://www.googleapis.com/auth/compute",<br>  "https://www.googleapis.com/auth/devstorage.full_control",<br>  "https://www.googleapis.com/auth/logging.write"<br>]</pre> | no |
| <a name="input_service_account_email"></a> [service\_account\_email](#input\_service\_account\_email) | The service account email to use. If null or 'default', then the default Compute Engine service account will be used. | `string` | `null` | no |
| <a name="input_shell_scripts"></a> [shell\_scripts](#input\_shell\_scripts) | A list of paths to local shell scripts which will be uploaded to customize the VM image | `list(string)` | `[]` | no |
| <a name="input_source_image"></a> [source\_image](#input\_source\_image) | Source OS image to build from | `string` | `null` | no |
| <a name="input_source_image_family"></a> [source\_image\_family](#input\_source\_image\_family) | Alternative to source\_image. Specify image family to build from latest image in family | `string` | `"fedora-37-base-image"` | no |
| <a name="input_source_image_project_id"></a> [source\_image\_project\_id](#input\_source\_image\_project\_id) | A list of project IDs to search for the source image. Packer will search the<br>first project ID in the list first, and fall back to the next in the list,<br>until it finds the source image. | `list(string)` | `null` | no |
| <a name="input_ssh_username"></a> [ssh\_username](#input\_ssh\_username) | Username to use for SSH access to VM | `string` | `"packer"` | no |
| <a name="input_startup_script"></a> [startup\_script](#input\_startup\_script) | Startup script (as raw string) used to build the custom Linux VM image (overridden by var.startup\_script\_file if both are set) | `string` | `null` | no |
| <a name="input_startup_script_file"></a> [startup\_script\_file](#input\_startup\_script\_file) | File path to local shell script that will be used to customize the Linux VM image (overrides var.startup\_script) | `string` | `null` | no |
| <a name="input_state_timeout"></a> [state\_timeout](#input\_state\_timeout) | The time to wait for instance state changes, including image creation | `string` | `"10m"` | no |
| <a name="input_subnetwork_name"></a> [subnetwork\_name](#input\_subnetwork\_name) | Name of subnetwork in which to provision image building VM | `string` | n/a | yes |
| <a name="input_tags"></a> [tags](#input\_tags) | Assign network tags to apply firewall rules to VM instance | `list(string)` | `null` | no |
| <a name="input_use_iap"></a> [use\_iap](#input\_use\_iap) | Use IAP proxy when connecting by SSH | `bool` | `true` | no |
| <a name="input_use_os_login"></a> [use\_os\_login](#input\_use\_os\_login) | Use OS Login when connecting by SSH | `bool` | `false` | no |
| <a name="input_wrap_startup_script"></a> [wrap\_startup\_script](#input\_wrap\_startup\_script) | Wrap startup script with Packer-generated wrapper | `bool` | `true` | no |
| <a name="input_zone"></a> [zone](#input\_zone) | Cloud zone in which to provision image building VM | `string` | n/a | yes |

## Outputs

No outputs.
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
