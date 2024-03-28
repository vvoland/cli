package image

import (
	"context"

	"github.com/containerd/platforms"
	"github.com/distribution/reference"
	"github.com/docker/cli/cli"
	"github.com/docker/cli/cli/command"
	imagetypes "github.com/docker/docker/api/types/image"
	"github.com/spf13/cobra"
)

type convertArgs struct {
	Src                    string
	Dst                    string
	Platforms              []string
	NoAttestations         bool
	OnlyAvailablePlatforms bool
}

func NewConvertCommand(dockerCli command.Cli) *cobra.Command {
	var args convertArgs

	cmd := &cobra.Command{
		Use:   "convert [OPTIONS] <from> <to>",
		Short: "Convert multi-platform images",
		Args:  cli.ExactArgs(2),
		RunE: func(cmd *cobra.Command, posArgs []string) error {
			args.Src = posArgs[0]
			args.Dst = posArgs[1]
			return runConvert(cmd.Context(), dockerCli, args)
		},
		Aliases: []string{"convert"},
		Annotations: map[string]string{
			"aliases": "docker image convert, docker convert",
		},
	}

	flags := cmd.Flags()
	flags.StringArrayVar(&args.Platforms, "platforms", nil, "Include only the specified platforms in the destination image")
	flags.BoolVar(&args.NoAttestations, "no-attestations", false, "Do not include image attestations")
	flags.BoolVar(&args.OnlyAvailablePlatforms, "available", false, "Only include platforms locally available to the daemon")

	return cmd
}

func runConvert(ctx context.Context, dockerCLI command.Cli, args convertArgs) error {
	dstRef, err := reference.ParseNormalizedNamed(args.Dst)
	if err != nil {
		return err
	}

	opts := imagetypes.ConvertOptions{
		NoAttestations:         args.NoAttestations,
		OnlyAvailablePlatforms: args.OnlyAvailablePlatforms,
	}

	for _, platform := range args.Platforms {
		p, err := platforms.Parse(platform)
		if err != nil {
			return err
		}
		opts.Platforms = append(opts.Platforms, p)
	}

	dstRef = reference.TagNameOnly(dstRef)
	dstRefTagged := dstRef.(reference.NamedTagged)
	return dockerCLI.Client().ImageConvert(ctx, args.Src, dstRefTagged, opts)
}
