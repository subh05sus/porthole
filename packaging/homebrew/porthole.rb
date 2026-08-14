# Homebrew formula template, matching GoReleaser's standard `brews:`
# output shape (see .goreleaser.yaml). Not registered in any tap — this
# is a reference template for when a real release exists; GoReleaser
# would normally generate and publish this automatically into a
# subh05sus/homebrew-tap repository, but no such repository or release
# exists yet this session (local tooling only, nothing pushed).
#
# Every {{ PLACEHOLDER }} below must be filled in from a real release
# (version, and the SHA256 of each platform's release archive) before
# this could actually be used.
class Porthole < Formula
  desc "A port kill switch and service viewer for your terminal"
  homepage "https://github.com/subh05sus/porthole"
  version "{{ VERSION }}"
  license "MIT"

  on_macos do
    on_arm do
      url "https://github.com/subh05sus/porthole/releases/download/v{{ VERSION }}/porthole_{{ VERSION }}_darwin_arm64.tar.gz"
      sha256 "{{ SHA256_DARWIN_ARM64 }}"
    end
    on_intel do
      url "https://github.com/subh05sus/porthole/releases/download/v{{ VERSION }}/porthole_{{ VERSION }}_darwin_amd64.tar.gz"
      sha256 "{{ SHA256_DARWIN_AMD64 }}"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/subh05sus/porthole/releases/download/v{{ VERSION }}/porthole_{{ VERSION }}_linux_arm64.tar.gz"
      sha256 "{{ SHA256_LINUX_ARM64 }}"
    end
    on_intel do
      url "https://github.com/subh05sus/porthole/releases/download/v{{ VERSION }}/porthole_{{ VERSION }}_linux_amd64.tar.gz"
      sha256 "{{ SHA256_LINUX_AMD64 }}"
    end
  end

  def install
    bin.install "porthole"
  end

  test do
    system "#{bin}/porthole", "--version"
  end
end
