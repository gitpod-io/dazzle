<center><img height="200" src="https://github.com/32leaves/dazzle/raw/master/logo.png" /></center>

dazzle (not this [dazzle](https://dota2.gamepedia.com/Dazzle)) is a rather experimental, if not hackish, Docker image builder. Its goal is to build independent layers where a change to one layer does invalidate the ones sitting "above" it. To this end, dazzle uses black magic.

## How does it work?
dazzle has two main capabilities.
1. _build indepedent layers_: dazzle uses a special label in a Docker file to establish "boundaries of independence", or meta layers if you so like. Statements in the form of `LABEL dazzle/layer=somename` establish those bounds. All content prior to the first label is used as base image for the other layers. Come build-time, dazzle will split the Dockerfile at the label statement and build them individually. This prevents accidential cross-talk between the layers.
2. merge those layers into one image: to merge any two Docker images (not just those built using dazzle), dazzle uses the Docker tar export to extract the base image and all "addons" (i.e. images that are to be merged). It then manipulates the manifests and image configurations such that upon re-import a single image exists. The process is a bit of a hack and like black magic fragile, possibly error prone and needs a black cat or two to work.

## Would I want to use this?
Not ordinarily, no. But if you have some special caching requirements this might just do the trick.

## Limitations and caveats
Expect nothing to work unless you've tested it yourself. Things that won't work for certain are multi-stage builds using `dazzle build`. There's no limitation on merging images that were created using multi-stage builds though (as far as I'm aware).
