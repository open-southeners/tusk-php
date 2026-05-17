<?php

declare(strict_types=1);

namespace PhpFeatures\Php85;

// Pipe operator |>
// The pipe operator passes the result of the left-hand expression as the
// first argument to the right-hand callable.
function double(int $n): int
{
    return $n * 2;
}

function addOne(int $n): int
{
    return $n + 1;
}

function square(int $n): int
{
    return $n ** 2;
}

// 3 |> double(...) |> addOne(...) |> square(...)
// = square(addOne(double(3))) = square(addOne(6)) = square(7) = 49
$result = 3 |> double(...) |> addOne(...) |> square(...);

// Pipe with string operations
function trim2(string $s): string { return trim($s); }
function upper(string $s): string { return strtoupper($s); }
function exclaim(string $s): string { return $s . '!'; }

$greeting = '  hello world  ' |> trim2(...) |> upper(...) |> exclaim(...);

// Pipe with closures
$process = fn(int $n): int => $n * 3;
$format  = fn(int $n): string => "Result: $n";

$output = 5 |> $process |> $format;

// Pipe chained with method calls
class Pipeline {
    private mixed $value;

    public function __construct(mixed $initial)
    {
        $this->value = $initial;
    }

    public static function of(mixed $value): static
    {
        return new static($value);
    }

    public function pipe(callable $fn): static
    {
        $this->value = $fn($this->value);
        return $this;
    }

    public function get(): mixed
    {
        return $this->value;
    }
}

// Combined features — new in PHP 8.5
class DataProcessor {
    public function __construct(
        private readonly string $name,
    ) {}

    public function process(mixed $data): mixed
    {
        return $data;
    }
}

// Pipe with array operations
$numbers = [1, 2, 3, 4, 5];
$sumDoubled = $numbers
    |> fn(array $arr): array => array_map(fn(int $n): int => $n * 2, $arr)
    |> fn(array $arr): int => array_sum($arr);
