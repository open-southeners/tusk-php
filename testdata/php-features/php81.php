<?php

declare(strict_types=1);

namespace PhpFeatures\Php81;

use Stringable;

// Pure enum
enum Direction {
    case North;
    case South;
    case East;
    case West;

    public function opposite(): Direction
    {
        return match($this) {
            Direction::North => Direction::South,
            Direction::South => Direction::North,
            Direction::East  => Direction::West,
            Direction::West  => Direction::East,
        };
    }

    public function label(): string
    {
        return match($this) {
            Direction::North => 'North',
            Direction::South => 'South',
            Direction::East  => 'East',
            Direction::West  => 'West',
        };
    }
}

// Backed enum (string)
enum Status: string {
    case Active   = 'active';
    case Inactive = 'inactive';
    case Pending  = 'pending';

    public function label(): string
    {
        return ucfirst($this->value);
    }

    public static function fromLabel(string $label): self
    {
        return self::from(strtolower($label));
    }
}

// Backed enum (int)
enum Priority: int {
    case Low    = 1;
    case Medium = 5;
    case High   = 10;

    const DEFAULT = self::Medium;

    public function isHigherThan(Priority $other): bool
    {
        return $this->value > $other->value;
    }
}

// Enum implementing interface
interface HasLabel {
    public function label(): string;
}

enum Color: string implements HasLabel {
    case Red   = 'red';
    case Green = 'green';
    case Blue  = 'blue';

    public function label(): string
    {
        return ucfirst($this->value);
    }

    public function hex(): string
    {
        return match($this) {
            Color::Red   => '#FF0000',
            Color::Green => '#00FF00',
            Color::Blue  => '#0000FF',
        };
    }
}

// Readonly properties
class ImmutablePoint {
    public readonly float $x;
    public readonly float $y;

    public function __construct(float $x, float $y)
    {
        $this->x = $x;
        $this->y = $y;
    }
}

class ImmutableRecord {
    public function __construct(
        public readonly int $id,
        public readonly string $name,
        public readonly \DateTimeImmutable $createdAt,
    ) {}

    public function withName(string $name): static
    {
        return new static($this->id, $name, $this->createdAt);
    }
}

// First-class callable syntax
function double(int $n): int
{
    return $n * 2;
}

$fn = double(...);
$arr = array_map(double(...), [1, 2, 3, 4, 5]);

class MathHelper {
    public function triple(int $n): int
    {
        return $n * 3;
    }

    public static function square(int $n): int
    {
        return $n ** 2;
    }
}

$helper = new MathHelper();
$tripleFn = $helper->triple(...);
$squareFn = MathHelper::square(...);
$strlenFn = strlen(...);

// never return type
function throwError(string $message): never
{
    throw new \RuntimeException($message);
}

function redirectAndExit(string $url): never
{
    header('Location: ' . $url);
    exit(0);
}

// Pure intersection types
interface Countable2 {
    public function count(): int;
}

interface Serializable2 {
    public function serialize(): string;
}

interface Loggable {
    public function log(): void;
}

function processCollection(Countable2&Serializable2 $collection): string
{
    return $collection->serialize();
}

function processLoggable(Loggable&Countable2 $item): int
{
    $item->log();
    return $item->count();
}

// new in initializers
class Logger {
    public function __construct(public readonly string $channel = 'default') {}
}

class Service {
    public function __construct(
        private Logger $logger = new Logger('service'),
        private array $config = [],
    ) {}
}

// Fibers
$fiber = new \Fiber(function (): void {
    $value = \Fiber::suspend('first');
    echo "Got: $value\n";
});

$result1 = $fiber->start();
$result2 = $fiber->resume('hello');

// Array unpacking with string keys
$array1 = ['a' => 1, 'b' => 2];
$array2 = ['c' => 3, 'd' => 4];
$merged = [...$array1, ...$array2];
