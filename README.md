<!--suppress ALL -->
<img height="310" alt="image" src="https://user-images.githubusercontent.com/16114089/121805566-0cd81280-cc4c-11eb-9b7d-b5f8a6db4f8d.png" align="right">

# Dragonfly

Dragonfly is a heavily asynchronous server software for Minecraft: Bedrock Edition written in Go. It was written with scalability
and simplicity in mind and aims to make the process of setting up a server and modifying it easy. Unlike other
Minecraft server software, Dragonfly is generally used as a library to extend.

[![Discord Banner 2](https://discordapp.com/api/guilds/623638955262345216/widget.png?style=banner2)](https://discord.gg/U4kFWHhTNR)

## Getting started
Running Dragonfly requires at least **Go 1.26**. After starting the server through one of the methods below,
**ctrl+c** may be used to shut down the server. Also check out the [wiki](https://github.com/df-mc/dragonfly/wiki) for
more detailed info.

#### Installation as library
```shell
go mod init github.com/user/module
go get github.com/df-mc/dragonfly
```

![SetupLibrary](https://user-images.githubusercontent.com/16114089/121804512-0f843900-cc47-11eb-9320-d195393b5a1f.gif)

#### Installation of the latest commit
```shell
git clone https://github.com/df-mc/dragonfly
cd dragonfly
go run main.go
```

![SetupClone](https://user-images.githubusercontent.com/16114089/121804495-ff6c5980-cc46-11eb-8e31-df4d94782e5b.gif)

## Developer info
[![Go Reference](https://pkg.go.dev/badge/github.com/df-mc/dragonfly/server.svg)](https://pkg.go.dev/github.com/df-mc/dragonfly/server)

Dragonfly features a well-documented codebase with an easy-to-use API. Documentation may be found
[here](https://pkg.go.dev/github.com/df-mc/dragonfly/server) and in the subpackages found by clicking *Directories*.

Publishing your project on GitHub? Consider adding the **[#df-mc](https://github.com/topic/df-mc)** topic to your
repository and opening a pull request at [df-wiki](https://github.com/df-mc/wiki) adding your project as a Community
Project to improve its visibility.

### Runtime resource pack replacement (fork)

`Server.ReloadResources(packs)` replaces the packs offered to new connections
without restarting the server. Active players must reconnect to negotiate the
new packs. Concurrent reload calls are serialised across the supporting listeners;
a connection negotiating during a reload may receive either snapshot.

Custom listeners can implement `server.ResourceReloadingListener`. Wrappers must
forward its `ReloadResources` method, or embed that capability when their wrapped
listener supports it. Embedding only `server.Listener` hides the optional method.
Implementations must snapshot the supplied slice and safely publish it to concurrent
connections. A nil or empty slice clears the offered packs. The server returns an
error if no listener supports replacement.

### Per-player swimming policy (fork)

Swimming remains enabled by default. Set `[Players] DisableSwimming = true` in
`config.toml`, or `server.Config.DisableSwimming = true`, to disable the swimming
pose for new connections. `player.Config.DisableSwimming` provides the same
initial policy when creating a player directly. On the player's world owner,
`p.SetSwimmingEnabled(false)` disables it and `p.SetSwimmingEnabled(true)` allows
the next swim request; `p.SwimmingEnabled()` reports the policy.

Disabling an active swim restores the normal pose, collider and eye height for
all viewers. Rejected legacy and modern swim inputs correct the controlling
client's swimming metadata. This is not an immobilisation or water-access rule:
ordinary water movement, crawling and gliding are unchanged. The policy follows
the handle across worlds but is intentionally omitted from `Player.Data()` and
the default player database. Reconnects use the configured default again;
gameplay may apply a per-player override after joining. No protocol layout or
version-specific adapter changes are required. Real-client pose prediction and
animation still need validation for each supported client family.

## Contributing
Contributions are very welcome! Issues, pull requests and feature requests are highly appreciated. Opening a pull
request? Consider joining our [Discord server](https://discord.gg/U4kFWHhTNR) to discuss your changes! Also have a read through the
[CONTRIBUTING.md](https://github.com/df-mc/dragonfly/blob/master/.github/CONTRIBUTING.md) for more info.
