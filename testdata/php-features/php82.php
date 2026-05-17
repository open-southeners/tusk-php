<?php

declare(strict_types=1);

namespace PhpFeatures\Php82;

// Readonly classes
readonly class Coordinate {
    public function __construct(
        public float $latitude,
        public float $longitude,
        public float $altitude = 0.0,
    ) {}

    public function distanceTo(Coordinate $other): float
    {
        // Haversine formula approximation
        $lat  = deg2rad($other->latitude - $this->latitude);
        $long = deg2rad($other->longitude - $this->longitude);
        $a = sin($lat / 2) ** 2 +
             cos(deg2rad($this->latitude)) *
             cos(deg2rad($other->latitude)) *
             sin($long / 2) ** 2;
        return 6371 * 2 * atan2(sqrt($a), sqrt(1 - $a));
    }
}

readonly class Money {
    public function __construct(
        public int $amount,
        public string $currency,
    ) {}

    public function add(Money $other): Money
    {
        if ($this->currency !== $other->currency) {
            throw new \InvalidArgumentException('Currency mismatch');
        }
        return new Money($this->amount + $other->amount, $this->currency);
    }

    public function multiply(int $factor): Money
    {
        return new Money($this->amount * $factor, $this->currency);
    }
}

// DNF (Disjunctive Normal Form) types
interface Stringable2 {
    public function __toString(): string;
}

interface JsonSerializable2 {
    public function jsonSerialize(): mixed;
}

interface Countable2 {
    public function count(): int;
}

function processDnf((Stringable2&JsonSerializable2)|null $value): string
{
    if ($value === null) {
        return '';
    }
    return (string) $value;
}

function handleCollection((Countable2&Stringable2)|array $collection): int
{
    if (is_array($collection)) {
        return count($collection);
    }
    return $collection->count();
}

class TypedContainer {
    private (Countable2&Stringable2)|null $inner = null;

    public function set((Countable2&Stringable2)|null $value): void
    {
        $this->inner = $value;
    }

    public function get(): (Countable2&Stringable2)|null
    {
        return $this->inner;
    }
}

// Standalone null, false, true types
function alwaysNull(): null
{
    return null;
}

function alwaysFalse(): false
{
    return false;
}

function alwaysTrue(): true
{
    return true;
}

function strictCheck(false|string $value): string
{
    if ($value === false) {
        return 'was false';
    }
    return $value;
}

function nullableStrict(null|int $value): int
{
    return $value ?? 0;
}

// Constants in traits
trait HasTimestamps {
    public const CREATED_AT = 'created_at';
    public const UPDATED_AT = 'updated_at';
    public const DELETED_AT = 'deleted_at';

    private ?\DateTimeImmutable $createdAt = null;
    private ?\DateTimeImmutable $updatedAt = null;

    public function touch(): void
    {
        $this->updatedAt = new \DateTimeImmutable();
    }

    public function getCreatedAt(): ?\DateTimeImmutable
    {
        return $this->createdAt;
    }
}

trait HasSoftDeletes {
    use HasTimestamps;

    public const SOFT_DELETE_COLUMN = self::DELETED_AT;

    private ?\DateTimeImmutable $deletedAt = null;

    public function delete(): void
    {
        $this->deletedAt = new \DateTimeImmutable();
    }

    public function isDeleted(): bool
    {
        return $this->deletedAt !== null;
    }
}

class Post {
    use HasSoftDeletes;

    public function __construct(
        public readonly string $title,
        public readonly string $content,
    ) {}
}

// Sensitive parameter attribute
function login(string $username, #[\SensitiveParameter] string $password): bool
{
    return $username === 'admin' && $password === 'secret';
}

// All-caps constants in traits
trait HasVersion {
    public const VERSION = '1.0.0';
    public const API_VERSION = 2;
}
