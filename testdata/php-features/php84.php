<?php

declare(strict_types=1);

namespace PhpFeatures\Php84;

// Property hooks — get and set
class Temperature {
    private float $celsius;

    public float $fahrenheit {
        get => ($this->celsius * 9 / 5) + 32;
        set => $this->celsius = ($value - 32) * 5 / 9;
    }

    public float $kelvin {
        get => $this->celsius + 273.15;
    }

    public function __construct(float $celsius)
    {
        $this->celsius = $celsius;
    }

    public function getCelsius(): float
    {
        return $this->celsius;
    }
}

class Circle {
    public float $radius {
        get => $this->_radius;
        set {
            if ($value < 0) {
                throw new \InvalidArgumentException('Radius must be non-negative');
            }
            $this->_radius = $value;
        }
    }

    private float $_radius;

    public float $area {
        get => M_PI * $this->_radius ** 2;
    }

    public float $circumference {
        get => 2 * M_PI * $this->_radius;
    }

    public function __construct(float $radius)
    {
        $this->radius = $radius;
    }
}

// Asymmetric visibility
class User {
    public private(set) int $id;
    public protected(set) string $name;
    protected private(set) string $email;

    public function __construct(int $id, string $name, string $email)
    {
        $this->id    = $id;
        $this->name  = $name;
        $this->email = $email;
    }

    public function updateName(string $name): void
    {
        $this->name = $name;
    }
}

class AdminUser extends User {
    public function changeEmail(string $email): void
    {
        $this->email = $email;
    }
}

// new without parentheses
class Container {
    private array $bindings = [];

    public function bind(string $abstract, string $concrete): void
    {
        $this->bindings[$abstract] = $concrete;
    }
}

$container = new Container;
$container->bind('foo', 'bar');

class EventDispatcher {
    private array $listeners = [];

    public function listen(string $event, callable $callback): void
    {
        $this->listeners[$event][] = $callback;
    }
}

$dispatcher = new EventDispatcher;

// #[\Deprecated] attribute
class LegacyHelper {
    #[\Deprecated(message: 'Use processNew() instead', since: '2.0')]
    public function processOld(string $input): string
    {
        return $this->processNew($input);
    }

    public function processNew(string $input): string
    {
        return strtoupper($input);
    }

    #[\Deprecated]
    public static function legacyCreate(array $data): static
    {
        return new static();
    }
}

#[\Deprecated(message: 'Use NewApi class instead', since: '3.0')]
class OldApi {
    public function request(string $endpoint): mixed
    {
        return null;
    }
}

// New array functions (8.4)
$numbers = [3, 1, 4, 1, 5, 9, 2, 6];
$first = array_find($numbers, fn(int $n): bool => $n > 4);
$firstKey = array_find_key($numbers, fn(int $n): bool => $n > 4);
$any = array_any($numbers, fn(int $n): bool => $n > 8);
$all = array_all($numbers, fn(int $n): bool => $n > 0);

// Lazy objects
class HeavyResource {
    private bool $initialized = false;

    public function __construct()
    {
        $this->initialized = true;
    }

    public function getData(): array
    {
        return ['initialized' => $this->initialized];
    }
}

// Chained method calls on new without parentheses
class Builder {
    private array $parts = [];

    public function add(string $part): static
    {
        $this->parts[] = $part;
        return $this;
    }

    public function build(): string
    {
        return implode(', ', $this->parts);
    }
}

$result = (new Builder)->add('a')->add('b')->build();

// readonly promoted properties with hooks in interfaces
interface Readable {
    public string $name { get; }
}
