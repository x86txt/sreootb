# Puppet manifest for CockroachDB single node setup (secure mode)
# Adjust node definitions in your Puppet site manifest for multi-node deployment

$cockroach_version = 'v23.2.4'
$cockroach_url     = "https://binaries.cockroachdb.com/cockroach-${cockroach_version}.linux-amd64.tgz"
$data_dir          = '/opt/sqldata'
$certs_dir         = '/opt/cockroach-certs'
$bin_path          = '/usr/local/bin/cockroach'
$cockroach_user    = 'cockroach'
$cockroach_group   = 'cockroach'
$mem_limit         = '1G'
$cpu_quota         = '50%'

# Create group and user
user { $cockroach_user:
  ensure     => present,
  system     => true,
  home       => $data_dir,
  shell      => '/sbin/nologin',
  managehome => false,
}

group { $cockroach_group:
  ensure => present,
}

# Ensure data directory
file { $data_dir:
  ensure  => directory,
  owner   => $cockroach_user,
  group   => $cockroach_group,
  mode    => '0700',
}

# Ensure certs directory
file { $certs_dir:
  ensure  => directory,
  owner   => $cockroach_user,
  group   => $cockroach_group,
  mode    => '0700',
}

# Place CA, node, and key files (assume distributed out-of-band, e.g. via scp or file resource)
file { "$certs_dir/ca.crt":
  ensure => file,
  owner  => $cockroach_user,
  group  => $cockroach_group,
  mode   => '0600',
}
file { "$certs_dir/node.crt":
  ensure => file,
  owner  => $cockroach_user,
  group  => $cockroach_group,
  mode   => '0600',
}
file { "$certs_dir/node.key":
  ensure => file,
  owner  => $cockroach_user,
  group  => $cockroach_group,
  mode   => '0600',
}

# Download and install CockroachDB
exec { 'download_cockroach':
  command => "/usr/bin/curl -sSL ${cockroach_url} -o /tmp/cockroach.tgz",
  creates => '/tmp/cockroach.tgz',
}

exec { 'extract_cockroach':
  command => '/bin/tar -xzf /tmp/cockroach.tgz -C /tmp',
  creates => "/tmp/cockroach-${cockroach_version}.linux-amd64/cockroach",
  require => Exec['download_cockroach'],
}

file { $bin_path:
  ensure  => file,
  owner   => 'root',
  group   => 'root',
  mode    => '0755',
  source  => "file:///tmp/cockroach-${cockroach_version}.linux-amd64/cockroach",
  require => Exec['extract_cockroach'],
}

# Systemd service (secure mode)
file { '/etc/systemd/system/cockroach.service':
  ensure  => file,
  owner   => 'root',
  group   => 'root',
  mode    => '0644',
  content => @(END),
[Unit]
Description=CockroachDB node
After=network.target

[Service]
Type=notify
User=${cockroach_user}
Group=${cockroach_group}
ExecStart=${bin_path} start \
  --certs-dir=${certs_dir} \
  --advertise-addr=${::ipaddress} \
  --listen-addr=${::ipaddress}:26257 \
  --http-addr=${::ipaddress}:8080 \
  --join=10.28.0.4:26257,10.28.0.3:26257,10.28.0.2:26257 \
  --cache=256MiB \
  --max-sql-memory=256MiB \
  --store=${data_dir} \
  --background
Restart=always
LimitNOFILE=35000
MemoryMax=${mem_limit}
CPUQuota=${cpu_quota}
Environment=COCKROACH_SKIP_ENABLING_DIAGNOSTIC_REPORTING=1

[Install]
WantedBy=multi-user.target
| END
  require => File[$bin_path],
}

exec { 'systemd_reload_cockroach':
  command     => '/bin/systemctl daemon-reload',
  refreshonly => true,
  subscribe   => File['/etc/systemd/system/cockroach.service'],
}

service { 'cockroach':
  ensure    => running,
  enable    => true,
  require   => [File['/etc/systemd/system/cockroach.service'], File[$data_dir], File[$certs_dir]],
  subscribe => File['/etc/systemd/system/cockroach.service'],
}

# Notify resource to echo the client cert (informational)
notify { 'client_cert':
  message => "Client certificate for SQL access (client.root.crt):\n" + file('/opt/cockroach-certs/client.root.crt'),
  require => File['/opt/cockroach-certs/client.root.crt'],
}

# Note: Ensure client.root.crt is distributed to /opt/cockroach-certs/client.root.crt on the node, or adjust path as needed. 