# Homebrew formula template, matching GoReleaser's *real, verified*
# `brews:` output shape (see .goreleaser.yaml) — this file was rewritten
# after actually running `goreleaser release --snapshot --clean` and
# inspecting dist/homebrew/Formula/porthole.rb, replacing an earlier,
# less accurate hand-written guess. Not registered in any tap; GoReleaser
# generates and publishes the real thing automatically into
# subh05sus/homebrew-tap once that repository exists and a real release
# runs — this file is a reference only, for eyeballing without running a
# release first, and needs every {{ PLACEHOLDER }} filled in to actually
# be usable directly. One deliberate addition beyond GoReleaser's raw
# output: the `test do` block at the bottom — GoReleaser doesn't generate
# one, but `brew audit`/`brew test-bot` expect it for a real submission.
#
# typed: false
# frozen_string_literal: true

class Porthole < Formula
  desc "A port kill switch and service viewer for your terminal."
  homepage "https://github.com/subh05sus/porthole"
  version "{{ VERSION }}"
  license "MIT"

  on_macos do
    if Hardware::CPU.intel?
      url "https://github.com/subh05sus/porthole/releases/download/v{{ VERSION }}/porthole_{{ VERSION }}_darwin_amd64.tar.gz"
      sha256 "{{ SHA256_DARWIN_AMD64 }}"

      def install
        bin.install "porthole"
      end
    end
    if Hardware::CPU.arm?
      url "https://github.com/subh05sus/porthole/releases/download/v{{ VERSION }}/porthole_{{ VERSION }}_darwin_arm64.tar.gz"
      sha256 "{{ SHA256_DARWIN_ARM64 }}"

      def install
        bin.install "porthole"
      end
    end
  end

  on_linux do
    if Hardware::CPU.intel? && Hardware::CPU.is_64_bit?
      url "https://github.com/subh05sus/porthole/releases/download/v{{ VERSION }}/porthole_{{ VERSION }}_linux_amd64.tar.gz"
      sha256 "{{ SHA256_LINUX_AMD64 }}"

      def install
        bin.install "porthole"
      end
    end
    if Hardware::CPU.arm? && Hardware::CPU.is_64_bit?
      url "https://github.com/subh05sus/porthole/releases/download/v{{ VERSION }}/porthole_{{ VERSION }}_linux_arm64.tar.gz"
      sha256 "{{ SHA256_LINUX_ARM64 }}"

      def install
        bin.install "porthole"
      end
    end
  end

  test do
    system "#{bin}/porthole", "--version"
  end
end
