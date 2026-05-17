<?php

declare(strict_types=1);

namespace App\Services;

use App\Models\User;

final class UserService {
    /** @var User[] */
    private array $users = [];

    public function all(): array
    {
        return $this->users;
    }

    public function find(int $id): ?User
    {
        return $this->users[$id] ?? null;
    }

    public function create(string $name, string $email): User
    {
        $user = new User(name: $name, email: $email);
        $this->users[] = $user;
        return $user;
    }

    public function update(int $id, string $name, string $email): ?User
    {
        $user = $this->find($id);

        if ($user === null) {
            return null;
        }

        return new User(name: $name, email: $email);
    }

    public function delete(int $id): bool
    {
        if (!isset($this->users[$id])) {
            return false;
        }

        unset($this->users[$id]);
        return true;
    }

    public function count(): int
    {
        return count($this->users);
    }
}
