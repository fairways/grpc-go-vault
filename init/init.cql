CREATE KEYSPACE if not exists identity
WITH replication = {'class':'SimpleStrategy','replication_factor': 1};

CREATE table if not exists identity.users(id uuid primary key, first_name varchar);