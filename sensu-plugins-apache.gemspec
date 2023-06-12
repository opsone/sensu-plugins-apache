# frozen_string_literal: true

require_relative 'lib/sensu/plugins/apache'

Gem::Specification.new do |spec|
  spec.name = 'sensu-plugins-apache'
  spec.version = Sensu::Plugins::Apache::VERSION
  spec.authors = ['Kevyn Lebouille']
  spec.email = ['kevyn.lebouille@opsone.net']

  spec.summary = 'This plugin provides facilities for monitoring Apache'
  spec.homepage = 'https://github.com/opsone/sensu-plugins-apache'
  spec.required_ruby_version = '>= 3.2.0'

  spec.metadata['allowed_push_host'] = "TODO: Set to your gem server 'https://example.com'"

  spec.metadata['homepage_uri'] = spec.homepage
  spec.metadata['source_code_uri'] = 'https://github.com/opsone/sensu-plugins-apache'
  spec.metadata['changelog_uri'] = 'https://github.com/opsone/sensu-plugins-apache/CHANGELOG.md'

  # Specify which files should be added to the gem when it is released.
  # The `git ls-files -z` loads the files in the RubyGem that have been added into git.
  spec.files = Dir.chdir(__dir__) do
    `git ls-files -z`.split("\x0").reject do |f|
      (File.expand_path(f) == __FILE__) || f.start_with?(*%w[test/ spec/ features/ .git .circleci appveyor])
    end
  end
  spec.bindir = 'bin'
  spec.executables = spec.files.grep(%r{\Abin/}) { |f| File.basename(f) }
  spec.require_paths = ['lib']

  # Uncomment to register a new dependency of your gem
  spec.add_dependency 'sensu-plugin', '~> 4.0'

  # For more information and examples about making a new gem, check out our
  # guide at: https://bundler.io/guides/creating_gem.html
end
