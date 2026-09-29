CREATE TABLE users (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    email varchar(254) NOT NULL CHECK (email = lower(btrim(email)) AND email <> ''),
    name varchar(100) NOT NULL CHECK (name = btrim(name) AND name <> '')
);

CREATE UNIQUE INDEX users_email_unique ON users (lower(email));
