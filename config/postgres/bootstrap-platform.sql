\set ON_ERROR_STOP on

\getenv postgres_admin_password POSTGRES_PASSWORD
\getenv gateway_migrator_password GATEWAY_MIGRATOR_PASSWORD
\getenv gateway_runtime_password GATEWAY_RUNTIME_PASSWORD

SELECT
  length(:'postgres_admin_password') >= 16
  AND length(:'gateway_migrator_password') >= 16
  AND length(:'gateway_runtime_password') >= 16
  AND :'postgres_admin_password' <> :'gateway_migrator_password'
  AND :'postgres_admin_password' <> :'gateway_runtime_password'
  AND :'gateway_migrator_password' <> :'gateway_runtime_password'
  AS local_credentials_are_valid
\gset

\if :local_credentials_are_valid
\else
  \echo 'ERROR: Local PostgreSQL credentials must be at least 16 characters and pairwise distinct.'
  \quit 3
\endif

SELECT format(
  'CREATE ROLE %I LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT NOREPLICATION NOBYPASSRLS PASSWORD %L',
  'gateway_migrator',
  :'gateway_migrator_password'
)
WHERE NOT EXISTS (
  SELECT 1
  FROM pg_roles
  WHERE rolname = 'gateway_migrator'
)
\gexec

SELECT format(
  'CREATE ROLE %I LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT NOREPLICATION NOBYPASSRLS PASSWORD %L',
  'gateway_runtime',
  :'gateway_runtime_password'
)
WHERE NOT EXISTS (
  SELECT 1
  FROM pg_roles
  WHERE rolname = 'gateway_runtime'
)
\gexec

ALTER ROLE gateway_migrator
  WITH LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT NOREPLICATION
  NOBYPASSRLS CONNECTION LIMIT -1 PASSWORD :'gateway_migrator_password'
  VALID UNTIL 'infinity';
ALTER ROLE gateway_runtime
  WITH LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT NOREPLICATION
  NOBYPASSRLS CONNECTION LIMIT -1 PASSWORD :'gateway_runtime_password'
  VALID UNTIL 'infinity';

SELECT format('REVOKE %I FROM %I', granted_role.rolname, member_role.rolname)
FROM pg_auth_members memberships
JOIN pg_roles granted_role ON granted_role.oid = memberships.roleid
JOIN pg_roles member_role ON member_role.oid = memberships.member
WHERE member_role.rolname IN ('gateway_migrator', 'gateway_runtime')
\gexec

SELECT format('CREATE DATABASE %I OWNER %I', 'platform', current_user)
WHERE NOT EXISTS (
  SELECT 1
  FROM pg_database
  WHERE datname = 'platform'
)
\gexec

ALTER DATABASE platform OWNER TO xdenovo_bootstrap;
REVOKE ALL PRIVILEGES ON DATABASE platform FROM PUBLIC;
REVOKE ALL PRIVILEGES ON DATABASE platform
  FROM gateway_migrator, gateway_runtime;
GRANT CONNECT, CREATE ON DATABASE platform TO gateway_migrator;
GRANT CONNECT ON DATABASE platform TO gateway_runtime;
