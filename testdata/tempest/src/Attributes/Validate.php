<?php

declare(strict_types=1);

namespace App\Attributes;

use Attribute;

#[Attribute(Attribute::TARGET_PROPERTY | Attribute::TARGET_PARAMETER)]
final class Validate {
    public function __construct(
        public readonly string $rule,
        public readonly string $message = '',
    ) {}
}

#[Attribute(Attribute::TARGET_PROPERTY | Attribute::TARGET_PARAMETER)]
final class Required {
    public function __construct(
        public readonly string $message = 'This field is required.',
    ) {}
}

#[Attribute(Attribute::TARGET_PROPERTY | Attribute::TARGET_PARAMETER)]
final class MinLength {
    public function __construct(
        public readonly int $length,
        public readonly string $message = '',
    ) {
        if ($this->message === '') {
            // Message will be generated at validation time
        }
    }
}

#[Attribute(Attribute::TARGET_PROPERTY | Attribute::TARGET_PARAMETER)]
final class MaxLength {
    public function __construct(
        public readonly int $length,
        public readonly string $message = '',
    ) {}
}

#[Attribute(Attribute::TARGET_PROPERTY | Attribute::TARGET_PARAMETER)]
final class Email {
    public function __construct(
        public readonly string $message = 'Please provide a valid email address.',
    ) {}
}
