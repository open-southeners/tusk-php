<?php

declare(strict_types=1);

namespace PhpFeatures\Php80;

// Union types
function processInput(int|string $value): int|string
{
    return $value;
}

// Named arguments
function createUser(string $name, int $age = 0, string $role = 'user'): array
{
    return compact('name', 'age', 'role');
}

$user = createUser(name: 'Alice', role: 'admin', age: 30);

// Constructor promotion
class Point {
    public function __construct(
        public readonly float $x,
        public readonly float $y,
        private float $z = 0.0,
    ) {}

    public function distanceTo(Point $other): float
    {
        return sqrt(
            ($this->x - $other->x) ** 2 +
            ($this->y - $other->y) ** 2 +
            ($this->z - $other->z) ** 2
        );
    }
}

// Nullsafe operator
class User {
    public ?Address $address = null;

    public function __construct(public string $name) {}
}

class Address {
    public ?City $city = null;

    public function __construct(public string $street) {}
}

class City {
    public function __construct(public string $name) {}

    public function getPostalCode(): ?string
    {
        return null;
    }
}

function getPostalCode(?User $user): ?string
{
    return $user?->address?->city?->getPostalCode();
}

// Match expression
function classify(mixed $value): string
{
    return match(true) {
        is_null($value)   => 'null',
        is_bool($value)   => 'boolean',
        is_int($value)    => 'integer',
        is_float($value)  => 'float',
        is_string($value) => 'string',
        is_array($value)  => 'array',
        default           => 'unknown',
    };
}

function httpStatus(int $code): string
{
    return match($code) {
        200 => 'OK',
        201 => 'Created',
        400 => 'Bad Request',
        401 => 'Unauthorized',
        403 => 'Forbidden',
        404 => 'Not Found',
        500 => 'Internal Server Error',
        default => 'Unknown',
    };
}

// Attributes
#[Attribute(Attribute::TARGET_CLASS | Attribute::TARGET_METHOD)]
class Route {
    public function __construct(
        public readonly string $path,
        public readonly string $method = 'GET',
    ) {}
}

#[Attribute(Attribute::TARGET_PROPERTY)]
class Column {
    public function __construct(
        public readonly string $name = '',
        public readonly string $type = 'string',
        public readonly bool $nullable = false,
    ) {}
}

#[Route('/users', method: 'GET')]
class UserController {
    #[Column(name: 'id', type: 'int')]
    private int $id = 0;

    #[Route('/users/{id}', method: 'POST')]
    public function store(int|string $id, string $name): array
    {
        return ['id' => $id, 'name' => $name];
    }
}

// Throw as expression
function assertNotEmpty(mixed $value): mixed
{
    return $value ?? throw new \InvalidArgumentException('Value must not be empty');
}

// str_contains, str_starts_with, str_ends_with
function stringHelpers(string $haystack): array
{
    return [
        'contains_php' => str_contains($haystack, 'php'),
        'starts_http'  => str_starts_with($haystack, 'http'),
        'ends_html'    => str_ends_with($haystack, '.html'),
    ];
}
