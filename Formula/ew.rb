class Ew < Formula
  desc "Fix failed shell commands and search command history using natural language"
  homepage "https://github.com/ashwch/ew"
  version "0.0.1"
  license "MIT"

  # This tracked file is a template/example.
  # Use scripts/render_formula.sh with release checksums to generate a publish-ready formula.
  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/ashwch/ew/releases/download/v0.0.1/ew_v0.0.1_darwin_arm64.tar.gz"
      sha256 "REPLACE_WITH_DARWIN_ARM64_SHA256"
    else
      url "https://github.com/ashwch/ew/releases/download/v0.0.1/ew_v0.0.1_darwin_amd64.tar.gz"
      sha256 "REPLACE_WITH_DARWIN_AMD64_SHA256"
    end
  end

  on_linux do
    if Hardware::CPU.arm?
      url "https://github.com/ashwch/ew/releases/download/v0.0.1/ew_v0.0.1_linux_arm64.tar.gz"
      sha256 "REPLACE_WITH_LINUX_ARM64_SHA256"
    else
      url "https://github.com/ashwch/ew/releases/download/v0.0.1/ew_v0.0.1_linux_amd64.tar.gz"
      sha256 "REPLACE_WITH_LINUX_AMD64_SHA256"
    end
  end

  def install
    bin.install "ew"
    bin.install "_ew"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/ew --version").strip
  end
end
