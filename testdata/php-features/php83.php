<?php

declare(strict_types=1);

namespace PhpFeatures\Php83;

// Typed class constants
interface Configurable {
    public const string DEFAULT_DRIVER = 'file';
    public const int MAX_CONNECTIONS = 100;
    public const bool DEBUG_MODE = false;
}

class CacheConfig implements Configurable {
    public const string DEFAULT_DRIVER = 'redis';
    public const int MAX_CONNECTIONS = 50;
    public const bool DEBUG_MODE = false;
    public const float TTL_SECONDS = 3600.0;
    public const array SUPPORTED_DRIVERS = ['file', 'redis', 'memcached'];
}

abstract class BaseRepository {
    public const string TABLE = '';
    protected const int BATCH_SIZE = 100;

    abstract public function find(int $id): ?object;
}

class UserRepository extends BaseRepository {
    public const string TABLE = 'users';
    protected const int BATCH_SIZE = 500;

    public function find(int $id): ?object
    {
        return null;
    }
}

// #[\Override] attribute
class ParentService {
    public function process(): string
    {
        return 'parent';
    }

    public function validate(): bool
    {
        return true;
    }
}

class ChildService extends ParentService {
    #[\Override]
    public function process(): string
    {
        return 'child: ' . parent::process();
    }

    #[\Override]
    public function validate(): bool
    {
        return parent::validate() && true;
    }
}

interface Processor {
    public function handle(mixed $input): mixed;
}

class ConcreteProcessor implements Processor {
    #[\Override]
    public function handle(mixed $input): mixed
    {
        return $input;
    }
}

// Dynamic class constant fetch
class Colors {
    public const string RED   = '#FF0000';
    public const string GREEN = '#00FF00';
    public const string BLUE  = '#0000FF';
}

function getColor(string $name): string
{
    return Colors::{$name};
}

class EnvConfig {
    public const string PROD = 'production';
    public const string DEV  = 'development';
    public const string TEST = 'testing';
}

function resolveEnv(): string
{
    $env = 'PROD';
    return EnvConfig::{$env};
}

// New json_validate function (8.3)
function validateJson(string $input): bool
{
    return json_validate($input);
}

// Typed constants in traits
trait HasMetadata {
    public const string METADATA_VERSION = '2.0';
    public const int METADATA_SCHEMA = 3;
}

class Document {
    use HasMetadata;

    public function __construct(
        public readonly string $title,
        public readonly string $body,
    ) {}

    public function getVersion(): string
    {
        return self::METADATA_VERSION;
    }
}

// readonly promoted properties in non-readonly class
class Config {
    public function __construct(
        public readonly string $dsn,
        public readonly int $timeout = 30,
        public readonly bool $persistent = false,
    ) {}
}

// Granular date/time exceptions
function parseDateSafe(string $input): ?\DateTimeImmutable
{
    try {
        return new \DateTimeImmutable($input);
    } catch (\DateMalformedStringException $e) {
        return null;
    }
}
