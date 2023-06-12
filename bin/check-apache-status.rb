#! /usr/bin/env ruby
# frozen_string_literal: true

require 'net/http'
require 'sensu-plugin/check/cli'

class CheckApacheStatus < Sensu::Plugin::Check::CLI
  option :hostname,
         short: '-h HOSTNAME',
         long: '--host HOSTNAME',
         description: 'Apache hostname',
         default: '127.0.0.1'

  option :port,
         short: '-p PORT',
         long: '--port PORT',
         description: 'Apache port',
         proc: proc(&:to_i),
         default: 80

  option :path,
         short: '-q STATUSPATH',
         long: '--path STATUSPATH',
         description: 'Path to your server status handler',
         default: 'apache-status'

  def run
    response = Net::HTTP.start(config[:hostname], config[:port]) do |connection|
      request = Net::HTTP::Get.new("/#{config[:path]}")
      connection.request(request)
    end

    ok 'Apache is Alive and healthy' if response.code == '200'
    warning 'Apache Status endpoint is mis-configured' if response.code == '404'
    critical "Apache is error #{response.code}"
  rescue StandardError
    critical 'Apache is Down'
  end
end
