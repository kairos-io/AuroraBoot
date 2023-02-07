<h1 align="center">
  <br>
     <img width="184" alt="kairos-white-column 5bc2fe34" src="https://user-images.githubusercontent.com/2420543/193010398-72d4ba6e-7efe-4c2e-b7ba-d3a826a55b7d.png"><br>
    AuroraBoot
<br>
</h1>

<h3 align="center">The Kairos bootstrapper</h3>
<p align="center">
  <a href="https://opensource.org/licenses/">
    <img src="https://img.shields.io/badge/licence-APL2-brightgreen"
         alt="license">
  </a>
  <a href="https://github.com/kairos-io/AuroraBoot/issues"><img src="https://img.shields.io/github/issues/kairos-io/AuroraBoot"></a>
  <a href="https://kairos.io/docs/" target=_blank> <img src="https://img.shields.io/badge/Documentation-blue"
         alt="docs"></a>
  <img src="https://img.shields.io/badge/made%20with-Go-blue">
  <img src="https://goreportcard.com/badge/github.com/kairos-io/AuroraBoot" alt="go report card" />
</p>


With Kairos you can build immutable, bootable Kubernetes and OS images for your edge devices as easily as writing a Dockerfile. Optional P2P mesh with distributed ledger automates node bootstrapping and coordination. Updating nodes is as easy as CI/CD: push a new image to your container registry and let secure, risk-free A/B atomic upgrades do the rest.


<table>
<tr>
<th align="center">
<img width="640" height="1px">
<p> 
<small>
Documentation
</small>
</p>
</th>
<th align="center">
<img width="640" height="1">
<p> 
<small>
Contribute
</small>
</p>
</th>
</tr>
<tr>
<td>

 📚 [Getting started with Kairos](https://kairos.io/docs/getting-started) <br> :bulb: [Examples](https://kairos.io/docs/examples) <br> :movie_camera: [Video](https://kairos.io/docs/media/) <br> :open_hands:[Engage with the Community](https://kairos.io/community/)
  
</td>
<td>
  
🙌[ CONTRIBUTING.md ]( https://github.com/kairos-io/kairos/blob/master/CONTRIBUTING.md ) <br> :raising_hand: [ GOVERNANCE ]( https://github.com/kairos-io/kairos/blob/master/GOVERNANCE.md ) <br>:construction_worker:[Code of conduct](https://github.com/kairos-io/kairos/blob/master/CODE_OF_CONDUCT.md) 
  
</td>
</tr>
</table>


## Description

`AuroraBoot` is an automatic boostrapper for `Kairos`:

- **Download** release assets in order to provision a machine
- **Prepare** automatically the environment to boot from network
- **Provision** machines from network with a version of Kairos and cloud config
- **Customize** The installation media for installations from USB

## Usage

`AuroraBoot` can be used with its container image to provision machines on the same network that will attempt to netboot. 

For instance, in one machine from your workstation, you can run:

```bash
$ docker run --rm -ti --net host quay.io/kairos/auroraboot --set "artifact_version=v1.5.0" --set "release_version=v1.5.0" --set "flavor=rockylinux" --set repository="kairos-io/kairos" --cloud-config /....
```

And then start machines attempting to boot over network.

This command will:
- **Download all the needed artifacts**
- **Create a custom ISO with the cloud config attached to drive automated installations**
- **Provision Kairos from network, with the same settings**

### Use container images

Auroraboot can also boostrap nodes by using custom container images or [the official kairos releases](https://kairos.io/docs/reference/image_matrix/), for instance:

```
docker run -v /var/run/docker.sock:/var/run/docker.sock --rm -ti --net host quay.io/kairos/auroraboot --set container_image=docker://quay.io/kairos/core-rockylinux:v1.5.0
```

This command will:
- **Use the image in the docker daemon running in the local host to boot it over network**
- **Create a custom ISO with the cloud config attached to drive automated installations**
- **Provision Kairos from network, with the same settings**

### Pulling without docker

If you don't have a running docker daemon, Auroraboot can also pull directly from remotes, for instance:


```
docker run --rm -ti --net host quay.io/kairos/auroraboot --set container_image=quay.io/kairos/core-rockylinux:v1.5.0
```

This command will:
- **Pull an image remotely to boot it over network**
- **Create a custom ISO with the cloud config attached to drive automated installations**
- **Provision Kairos from network, with the same settings**

### Configuration

`AuroraBoot` takes configuration settings either from the CLI arguments or from a `YAML` configuration file.

A configuration file can be for instance:

```yaml
artifact_version: "v1.5.0"
release_version: "v1.5.0"
container_image: "..."
flavor: "rockylinux"
repository: "kairos-io/kairos"

cloud_config: |
```

Any field of the `YAML` file, excluding `cloud_config` can be configured with the `--set` argument in the CLI. 

**Note**

- Specyfing a `container_image` takes precedence over the specified artifacts.