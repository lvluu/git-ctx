# Changelog

## [0.3.0](https://github.com/lvluu/git-ctx/compare/v0.2.2...v0.3.0) (2026-03-28)


### Features

* add post-worktree hooks for automatic setup ([#10](https://github.com/lvluu/git-ctx/issues/10)) ([eacb628](https://github.com/lvluu/git-ctx/commit/eacb6285fbbc0a7f97b3c5bc8a9ec8fbc96a7e9a)), closes [#9](https://github.com/lvluu/git-ctx/issues/9)
* **doctor:** add --fix mode for auto-repair ([#28](https://github.com/lvluu/git-ctx/issues/28)) ([c9252a9](https://github.com/lvluu/git-ctx/commit/c9252a92d5a935f417819bf5e2c112756a6d4ccc))
* **profile:** add --dry-run and --diff to apply ([#26](https://github.com/lvluu/git-ctx/issues/26)) ([58536da](https://github.com/lvluu/git-ctx/commit/58536dad411c1b18cb97897f57d16004d559e7bb))
* **profile:** add --verbose and --json output to list ([#27](https://github.com/lvluu/git-ctx/issues/27)) ([1258bbc](https://github.com/lvluu/git-ctx/commit/1258bbcb8e97f5670749d435b7e9fedd0a7e35f5))
* **profile:** add diff command ([#24](https://github.com/lvluu/git-ctx/issues/24)) ([323a45f](https://github.com/lvluu/git-ctx/commit/323a45f043d4e9e7ca21041a23b8d32eeb66c798))
* **profile:** add gist export/import ([e1ed226](https://github.com/lvluu/git-ctx/commit/e1ed2267389cc9d672be12c5e20fe1863dc98719))
* **profile:** add GPG key management per profile ([#34](https://github.com/lvluu/git-ctx/issues/34)) ([ccd4181](https://github.com/lvluu/git-ctx/commit/ccd4181f173abaf128c7bb74a8bd0670801f2580))
* **profile:** add interactive switch command with fuzzy search ([#25](https://github.com/lvluu/git-ctx/issues/25)) ([40dbf15](https://github.com/lvluu/git-ctx/commit/40dbf1529f2f78e7f7315f3d8086a824fc01ebd3))
* **profile:** add smart auto-switch with remote, email, and path matchers ([#32](https://github.com/lvluu/git-ctx/issues/32)) ([2a3caf9](https://github.com/lvluu/git-ctx/commit/2a3caf95ab965ed7ba68fd815e917d2bcc89695f))
* **profile:** add template inheritance (extends) ([42b2254](https://github.com/lvluu/git-ctx/commit/42b22542acd799fcd300429c6864e55c8b6e4143)), closes [#13](https://github.com/lvluu/git-ctx/issues/13)
* **worktree:** add bidirectional sync and watch mode ([d3a7597](https://github.com/lvluu/git-ctx/commit/d3a759794af3b6feb5cb01bf425595055693e07a))
* **worktree:** add bidirectional sync and watch mode ([84fba4d](https://github.com/lvluu/git-ctx/commit/84fba4d9b0ed0240379d46529abb92209eb6bdaa)), closes [#14](https://github.com/lvluu/git-ctx/issues/14)


### Bug Fixes

* address CR comments from PR review ([67fe53d](https://github.com/lvluu/git-ctx/commit/67fe53d02eba30d04e57f4b0f0be659a603cb8aa))
* address remaining CR comments ([866f968](https://github.com/lvluu/git-ctx/commit/866f96808240b216320a50bc238116ad7f40587e))
* **ci:** add pull-requests write permission to release-please ([143faef](https://github.com/lvluu/git-ctx/commit/143faefd935328e96c4137aff1e8e510c2a04dae))
* **ci:** add release-type input to release-please action ([6d435ee](https://github.com/lvluu/git-ctx/commit/6d435ee80dbcc80fd46e3e592df727a488a452cc))
* **ci:** add write permissions to release-please and goreleaser jobs ([6f709e7](https://github.com/lvluu/git-ctx/commit/6f709e7104703c6eabf8c184e3bd921111fe662b))
* **ci:** skip creating pull request in release-please ([4352088](https://github.com/lvluu/git-ctx/commit/435208815afca372ea40ea2142b5087cadc2def4))
* correct fakeRunner key lookup in git tests ([435c187](https://github.com/lvluu/git-ctx/commit/435c1873b28cd25ab4a812f6126729465b23e8e7))
* use correct build path in e2e tests ([2338167](https://github.com/lvluu/git-ctx/commit/2338167a6d4439c6401b11f4e56d9fb9af56bdfb))
