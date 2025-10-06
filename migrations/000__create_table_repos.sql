CREATE TABLE repos (
    id BIGSERIAL PRIMARY KEY,

    owner VARCHAR(255) NOT NULL,
    name VARCHAR(255) NOT NULL,

    build_counter BIGINT NOT NULL DEFAULT 0,
    cache_id BIGINT DEFAULT NULL,

    UNIQUE (owner, name)
);
